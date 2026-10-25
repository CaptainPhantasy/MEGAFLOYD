#!/usr/bin/python3
"""Opt-in, per-user MegaFloyd login startup and work-volume mounting on macOS."""
import argparse
import ctypes as c
import datetime as dt
import json
import os
from pathlib import Path
import plistlib
import pwd
import re
import shutil
import subprocess
import sys
import time
import urllib.parse
import uuid

LABEL = 'ai.legacy.megafloyd.startup'
HOME = Path(pwd.getpwuid(os.getuid()).pw_dir)
ROOT = HOME / '.megafloyd' / 'startup'
CONFIG = ROOT / 'config.json'
PLIST = HOME / 'Library' / 'LaunchAgents' / (LABEL + '.plist')


def stamp():
    return dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')


def write_owned(path, data, mode=0o600):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    if path.exists():
        shutil.copy2(path, Path(str(path) + '.' + stamp() + '.bak-startup'))
    temp = path.with_name(path.name + '.' + str(os.getpid()) + '.tmp')
    with open(temp, 'xb') as out:
        os.chmod(temp, mode)
        out.write(data)
    os.replace(temp, path)


def remote_identity(value):
    url = urllib.parse.urlsplit(value)
    if url.scheme.lower() != 'smb' or not url.hostname or not url.path.strip('/'):
        raise ValueError('A network share must be an smb://host/share URL')
    if url.password or url.query or url.fragment:
        raise ValueError('Passwords, query strings, and fragments are not accepted in share URLs')
    return (url.scheme.lower(), url.hostname.lower(), url.port, urllib.parse.unquote(url.path).rstrip('/'))


def validate(config):
    if config.get('owner_uid') != os.getuid():
        raise ValueError('This startup configuration belongs to a different OS account')
    app = Path(config['app'])
    if not app.is_absolute() or app.name != 'MEGAFLOYD.app':
        raise ValueError('Set an absolute path to the canonical MEGAFLOYD.app')
    for item in config.get('volumes', []):
        if set(item) - {'label', 'uuid', 'path'}:
            raise ValueError('Unrecognized work-volume field')
        uuid.UUID(item['uuid'])
        if not Path(item['path']).is_absolute():
            raise ValueError('Work-volume paths must be absolute')
    for item in config.get('shares', []):
        if set(item) - {'label', 'url', 'username', 'path'}:
            raise ValueError('Unrecognized network-share field; credentials are not stored here')
        remote_identity(item['url'])
        if urllib.parse.urlsplit(item['url']).username:
            raise ValueError('Use the separate username field, never user information in a share URL')
        if not Path(item['path']).is_absolute():
            raise ValueError('Network mount paths must be absolute')
    return config


def install(args):
    mounts = json.loads(Path(args.mounts_json).read_text()) if args.mounts_json else {}
    if set(mounts) - {'volumes', 'shares'}:
        raise ValueError('Mount configuration supports only volumes and shares')
    config = validate({'version': 1, 'owner_uid': os.getuid(), 'app': str(Path(args.app).absolute()),
                       'volumes': mounts.get('volumes', []), 'shares': mounts.get('shares', [])})
    helper = ROOT / 'helper.py'
    ROOT.mkdir(parents=True, exist_ok=True, mode=0o700)
    write_owned(helper, Path(__file__).read_bytes(), 0o700)
    write_owned(CONFIG, (json.dumps(config, indent=2) + '\n').encode())
    agent = {
        'Label': LABEL,
        'ProgramArguments': ['/usr/bin/python3', str(helper), 'run'],
        'RunAtLoad': True,
        'KeepAlive': {'SuccessfulExit': False},
        'ThrottleInterval': 30,
        'LimitLoadToSessionType': 'Aqua',
        'ProcessType': 'Background',
        'Umask': 0o077,
        'StandardOutPath': str(ROOT / 'launchd.log'),
        'StandardErrorPath': str(ROOT / 'launchd-error.log'),
    }
    write_owned(PLIST, plistlib.dumps(agent))
    subprocess.run(['/usr/bin/plutil', '-lint', str(PLIST)], check=True, stdout=subprocess.DEVNULL)
    print(json.dumps({'installed': True, 'activated': False, 'helper': str(helper), 'config': str(CONFIG),
                      'launch_agent': str(PLIST), 'volume_count': len(config['volumes']),
                      'share_count': len(config['shares']),
                      'activation_command': 'launchctl bootstrap gui/' + str(os.getuid()) + ' ' + str(PLIST)}))


def frameworks():
    cf = c.CDLL('/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation')
    nf = c.CDLL('/System/Library/Frameworks/NetFS.framework/NetFS')
    cf.CFStringCreateWithCString.argtypes = [c.c_void_p, c.c_char_p, c.c_uint32]
    cf.CFStringCreateWithCString.restype = c.c_void_p
    cf.CFURLCreateWithString.argtypes = [c.c_void_p, c.c_void_p, c.c_void_p]
    cf.CFURLCreateWithString.restype = c.c_void_p
    cf.CFURLCreateFromFileSystemRepresentation.argtypes = [c.c_void_p, c.c_char_p, c.c_long, c.c_bool]
    cf.CFURLCreateFromFileSystemRepresentation.restype = c.c_void_p
    cf.CFURLGetString.argtypes = [c.c_void_p]
    cf.CFURLGetString.restype = c.c_void_p
    cf.CFStringGetCString.argtypes = [c.c_void_p, c.c_void_p, c.c_long, c.c_uint32]
    cf.CFStringGetCString.restype = c.c_bool
    cf.CFRelease.argtypes = [c.c_void_p]
    nf.NetFSCopyURLForRemountingVolume.argtypes = [c.c_void_p]
    nf.NetFSCopyURLForRemountingVolume.restype = c.c_void_p
    return cf, nf


def cf_text(cf, value):
    buf = c.create_string_buffer(16384)
    if not cf.CFStringGetCString(value, buf, len(buf), 0x08000100):
        raise ValueError('macOS returned an unreadable mount identifier')
    return buf.value.decode()


def mounted_share(path, expected_url):
    if not os.path.ismount(path) or not os.access(path, os.R_OK | os.X_OK):
        return False
    cf, nf = frameworks()
    encoded = os.fsencode(path)
    local = cf.CFURLCreateFromFileSystemRepresentation(None, encoded, len(encoded), True)
    remote = nf.NetFSCopyURLForRemountingVolume(local)
    try:
        return bool(remote) and remote_identity(cf_text(cf, cf.CFURLGetString(remote))) == remote_identity(expected_url)
    finally:
        if remote:
            cf.CFRelease(remote)
        cf.CFRelease(local)


def mount_network_worker(item):
    """NetFS uses the logged-in user's existing OS session/Keychain, without UI."""
    cf, nf = frameworks()
    cf.CFDictionaryCreateMutable.argtypes = [c.c_void_p, c.c_long, c.c_void_p, c.c_void_p]
    cf.CFDictionaryCreateMutable.restype = c.c_void_p
    cf.CFDictionarySetValue.argtypes = [c.c_void_p, c.c_void_p, c.c_void_p]
    nf.NetFSMountURLSync.argtypes = [c.c_void_p, c.c_void_p, c.c_void_p, c.c_void_p,
                                    c.c_void_p, c.c_void_p, c.POINTER(c.c_void_p)]
    nf.NetFSMountURLSync.restype = c.c_int
    cf.CFArrayGetCount.argtypes = [c.c_void_p]
    cf.CFArrayGetCount.restype = c.c_long
    cf.CFArrayGetValueAtIndex.argtypes = [c.c_void_p, c.c_long]
    cf.CFArrayGetValueAtIndex.restype = c.c_void_p
    owned = []
    def string(text):
        ref = cf.CFStringCreateWithCString(None, text.encode(), 0x08000100)
        owned.append(ref)
        return ref
    url = cf.CFURLCreateWithString(None, string(item['url']), None)
    owned.append(url)
    key_callbacks = c.addressof(c.c_byte.in_dll(cf, 'kCFTypeDictionaryKeyCallBacks'))
    value_callbacks = c.addressof(c.c_byte.in_dll(cf, 'kCFTypeDictionaryValueCallBacks'))
    options = cf.CFDictionaryCreateMutable(None, 0, key_callbacks, value_callbacks)
    owned.append(options)
    cf.CFDictionarySetValue(options, string('UIOption'), string('NoUI'))
    username = string(item['username']) if item.get('username') else None
    points = c.c_void_p()
    try:
        result = nf.NetFSMountURLSync(url, None, username, None, options, None, c.byref(points))
        paths = []
        if points:
            paths = [cf_text(cf, cf.CFArrayGetValueAtIndex(points, i)) for i in range(cf.CFArrayGetCount(points))]
        print(json.dumps({'status': result, 'mountpoints': paths}))
    finally:
        if points:
            cf.CFRelease(points)
        for ref in reversed(owned):
            if ref:
                cf.CFRelease(ref)


def command(args, deadline, limit=8):
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        raise TimeoutError('Startup attempt reached its time limit; launchd will retry')
    return subprocess.run(args, capture_output=True, timeout=min(limit, remaining))


def volume_state(item, deadline):
    # APFS may mount a snapshot while its base UUID reports no MountPoint.
    # Validate the visible mount first, then locate an absent drive by UUID.
    targets = [item['path'], item['uuid']] if os.path.ismount(item['path']) else [item['uuid']]
    for target in targets:
        result = command(['/usr/sbin/diskutil', 'info', '-plist', target], deadline, 3)
        if result.returncode:
            continue
        info = plistlib.loads(result.stdout)
        if info.get('VolumeUUID', '').upper() != item['uuid'].upper():
            continue
        if info.get('Internal') is not False:
            raise ValueError('Configured work-volume identity resolves to an internal disk')
        return info
    return None


def ensure_mounts(config, mutate):
    deadline = time.monotonic() + 50
    entries = []
    for item in config.get('volumes', []):
        entry = {'kind': 'volume', 'label': item.get('label', item['path']), 'uuid': item['uuid'], 'path': item['path'], 'mounted_now': False}
        try:
            info = volume_state(item, deadline)
            if info and not info.get('MountPoint') and mutate:
                result = command(['/usr/sbin/diskutil', 'mount', item['uuid']], deadline)
                entry['mount_attempted'] = True
                entry['mount_exit'] = result.returncode
                info = volume_state(item, deadline)
                entry['mounted_now'] = result.returncode == 0
            point = info.get('MountPoint') if info else None
            entry['actual_path'] = point
            entry['ready'] = bool(point and os.path.ismount(point) and os.access(point, os.R_OK | os.X_OK))
            if not entry['ready']:
                entry['action'] = 'Connect or unlock this external volume; macOS mounting will retry at login'
        except (OSError, ValueError, TimeoutError, subprocess.TimeoutExpired) as error:
            entry.update(ready=False, error=type(error).__name__, action='Check this volume in Disk Utility; startup will retry')
        entries.append(entry)
    for item in config.get('shares', []):
        entry = {'kind': 'share', 'label': item.get('label', item['path']), 'path': item['path'], 'mounted_now': False}
        try:
            ready = mounted_share(item['path'], item['url'])
            if not ready and mutate:
                # The separate worker bounds a native call that may wait for a network.
                remaining = max(0.1, min(18, deadline - time.monotonic()))
                if deadline <= time.monotonic():
                    raise TimeoutError()
                worker = subprocess.run(['/usr/bin/python3', str(Path(__file__).resolve()), '_mount-network'],
                                        input=json.dumps(item).encode(), capture_output=True, timeout=remaining)
                if worker.returncode == 0:
                    receipt = json.loads(worker.stdout)
                    entry['netfs_status'] = receipt['status']
                    candidates = [item['path']] + receipt.get('mountpoints', [])
                    point = next((p for p in candidates if mounted_share(p, item['url'])), None)
                    ready = point is not None
                    entry['actual_path'] = point
                    entry['mounted_now'] = ready
                else:
                    entry['worker_exit'] = worker.returncode
            entry['ready'] = ready
            if not ready:
                entry['action'] = 'Connect the network and mount this share once in Finder with this account, saving its credentials in Keychain; startup uses NoUI and will retry'
        except (OSError, ValueError, TimeoutError, subprocess.TimeoutExpired) as error:
            entry.update(ready=False, error=type(error).__name__, action='Check network reachability and this account\'s saved Finder/Keychain share access; startup will retry without an authentication prompt')
        entries.append(entry)
    return entries


def login_identity():
    boot = subprocess.run(['/usr/sbin/sysctl', '-n', 'kern.bootsessionuuid'], capture_output=True, text=True, timeout=5, check=True).stdout.strip()
    gui = subprocess.run(['/bin/launchctl', 'print', 'gui/' + str(os.getuid())], capture_output=True, text=True, timeout=5, check=True).stdout
    sessions = re.findall(r'^\s*asid = (\d+)\s*$', gui, re.MULTILINE)
    if not boot or len(sessions) != 1:
        raise ValueError('Cannot identify the current Aqua login session; startup will retry')
    return {'owner_uid': os.getuid(), 'boot_session': boot, 'aqua_audit_session': sessions[0]}


def save_status(path, data):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    temp = path.with_name(path.name + '.' + str(os.getpid()) + '.tmp')
    temp.write_text(json.dumps(data, indent=2) + '\n')
    os.chmod(temp, 0o600)
    os.replace(temp, path)


def run(args):
    config = validate(json.loads(CONFIG.read_text()))
    entries = ensure_mounts(config, args.command != 'check')
    ready = all(item['ready'] for item in entries)
    app = Path(config['app'])
    app_ready = (app / 'Contents' / 'MacOS' / 'MEGAFLOYD').is_file()
    receipt = {'checked_at': dt.datetime.now(dt.timezone.utc).isoformat(), 'owner_uid': os.getuid(),
               'mode': args.command, 'app': str(app), 'mounts': entries,
               'mounts_ready': ready, 'app_ready': app_ready, 'opened': False}
    opened_this_login = False
    if args.command == 'run':
        identity = login_identity()
        marker = ROOT / 'launched-session.json'
        previous = json.loads(marker.read_text()) if marker.exists() else None
        opened_this_login = previous == identity
        receipt['app_launch_suppressed'] = opened_this_login
        # Optional absent drives/shares keep retrying without withholding an
        # available application or reopening one the user already closed.
        if app_ready and not opened_this_login:
            opened = subprocess.run(['/usr/bin/open', '-a', str(app)], capture_output=True, timeout=15)
            receipt['launch_exit'] = opened.returncode
            receipt['opened'] = opened.returncode == 0
            if receipt['opened']:
                save_status(marker, identity)
                opened_this_login = True
        receipt['app_opened_this_login'] = opened_this_login
    success = ready and app_ready and (args.command != 'run' or opened_this_login)
    receipt['success'] = success
    if not success:
        receipt['next_action'] = 'launchd retries failed startup attempts every 30 seconds; inspect this account\'s ~/.megafloyd/startup/status.json'
    if args.command != 'check':
        save_status(ROOT / 'status.json', receipt)
    print(json.dumps(receipt, indent=2))
    return 0 if success else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    setup = sub.add_parser('install', help='Opt in this OS account; stage a login LaunchAgent without starting it now')
    setup.add_argument('--app', required=True)
    setup.add_argument('--mounts-json', help='Own JSON with volumes [{uuid,path,label}] and shares [{url,username,path,label}], no passwords')
    sub.add_parser('check', help='Read mount and app readiness without mounting, opening, or writing status')
    sub.add_parser('mount-only', help='Mount configured work locations without opening the app')
    sub.add_parser('run', help='Mount configured work locations, then open the canonical app once')
    sub.add_parser('_mount-network', help=argparse.SUPPRESS)
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise ValueError('This login startup helper requires macOS')
    if args.command == 'install':
        install(args)
        return 0
    if args.command == '_mount-network':
        mount_network_worker(json.load(sys.stdin))
        return 0
    return run(args)


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception as error:
        print(json.dumps({'success': False, 'error': type(error).__name__, 'message': str(error)}), file=sys.stderr)
        sys.exit(1)
