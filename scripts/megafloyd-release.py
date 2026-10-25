#!/usr/bin/env python3
"""Build and atomically install complete MegaFloyd releases. Python standard library only."""
import argparse
import datetime
import fcntl
import hashlib
import json
import os
import pathlib
import plistlib
import platform
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile

CDN = "https://inference.tail58d565.ts.net:8444/megafloyd"
PRODUCT = "megafloyd"
ARCHIVE_ROOT = "megafloyd-release"
APP = "MEGAFLOYD.app"
RESOURCES = APP + "/Contents/Resources/megafloyd"
NATIVE = RESOURCES + "/bin/megafloyd"
WEB = RESOURCES + "/web"
DESTINATION_ERROR = "Error: No destination folder specified. Supply an output path within your user space."


def timestamp():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")


def sha256(path):
    result = hashlib.sha256()
    with open(path, "rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def encoded(value):
    return (json.dumps(value, sort_keys=True, indent=2) + "\n").encode()


def digest(value):
    return hashlib.sha256(encoded(value)).hexdigest()


def safe_name(value):
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]*", value):
        raise ValueError("invalid release identity")
    return value


def relative(value):
    if not isinstance(value, str) or not value or "\\" in value:
        raise ValueError("invalid relative package path")
    candidate = pathlib.PurePosixPath(value)
    if candidate.is_absolute() or ".." in candidate.parts or "." in candidate.parts:
        raise ValueError("package path escapes its release")
    return candidate


def inside(root, path):
    try:
        path.resolve().relative_to(root.resolve())
    except ValueError:
        raise ValueError("package link escapes its release: " + str(path)) from None


def inventory(root):
    records = []
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in sorted(dirs + files):
            path = pathlib.Path(directory) / name
            rel = path.relative_to(root).as_posix()
            if rel == "release.json":
                continue
            info = path.lstat()
            mode = stat.S_IMODE(info.st_mode)
            if stat.S_ISLNK(info.st_mode):
                inside(root, path)
                if not path.exists():
                    raise ValueError("package has a broken link: " + rel)
                records.append({"path": rel, "type": "symlink", "target": os.readlink(path)})
            elif stat.S_ISREG(info.st_mode):
                records.append({"path": rel, "type": "file", "mode": mode,
                                "size": info.st_size, "sha256": sha256(path)})
            elif stat.S_ISDIR(info.st_mode):
                records.append({"path": rel, "type": "directory", "mode": mode})
            else:
                raise ValueError("unsupported package entry: " + rel)
    return sorted(records, key=lambda item: item["path"])


def run(args, **kwargs):
    return subprocess.run(args, check=True, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, text=True, timeout=60, **kwargs).stdout.strip()


def host_target():
    system = platform.system().lower()
    arch = {"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64"}.get(platform.machine())
    if arch is None:
        raise ValueError("unsupported machine architecture: " + platform.machine())
    return system + "-" + arch


def source_receipt(root):
    paths = subprocess.check_output(["git", "-C", str(root), "ls-files", "-z", "--cached", "--others", "--exclude-standard"])
    selected = []
    for raw in sorted(set(paths.split(b"\0"))):
        if not raw:
            continue
        rel = raw.decode()
        if ".bak" in rel or rel.startswith(".tmp/"):
            continue
        source = (rel.startswith("floyd-go/") or rel.startswith("scripts/") or
                  rel.startswith("desktop/desktop/") or
                  (rel.startswith("floyd-code/") and ("/src/" in rel or rel.endswith("pnpm-lock.yaml"))))
        path = root / rel
        if source and path.is_file() and path.suffix in {".go", ".sql", ".mod", ".sum", ".json", ".ts", ".tsx", ".js", ".mjs", ".sh", ".py", ".yaml", ".yml", ".md", ".tpl", ".html", ".icns"}:
            selected.append({"path": rel, "sha256": sha256(path)})
    return {"commit": run(["git", "-C", str(root), "rev-parse", "HEAD"]),
            "dirty": bool(run(["git", "-C", str(root), "status", "--porcelain"])),
            "input_count": len(selected), "inputs_sha256": digest(selected)}


RELEASE_LAUNCHER = '''#!/bin/sh
set -eu
release=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
platform="$release/MEGAFLOYD.app/Contents/Resources/megafloyd"
export MEGAFLOYD_RELEASE_ID="{release_id}"
export MEGAFLOYD_VERSION="{version}"
export MEGAFLOYD_BUILD_ID="{build_id}"
export MEGAFLOYD_ACTIVE=1
export MEGAFLOYD_WEB_SCRIPT="$platform/web/dist/main.mjs"
export MEGAFLOYD_WEB_BUNDLE="$platform/web/dist-web"
export MEGAFLOYD_WEB_NODE="$platform/bin/node"
exec "$platform/bin/megafloyd" "$@"
'''

PREFIX_LAUNCHER = '''#!/bin/sh
set -eu
prefix=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
release=$(CDPATH= cd -- "$prefix/current" && pwd -P)
if [ "${1:-}" = update ]; then
  shift
  exec python3 "$release/install.py" install --prefix "$prefix" "$@"
fi
exec "$release/bin/megafloyd" "$@"
'''


def package(args):
    version = safe_name(args.version)
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        raise ValueError("release version must contain three numeric parts")
    build_id = safe_name(args.build_id)
    release_id = version + "-" + build_id
    target = host_target()
    if target != "darwin-arm64":
        raise ValueError("this desktop package currently builds on darwin-arm64")
    root = pathlib.Path(args.repo).resolve()
    app = pathlib.Path(args.app).resolve()
    out = pathlib.Path(args.out).resolve()
    if out.exists():
        raise ValueError("release output already exists; choose a new output directory")
    out.mkdir(parents=True)
    payload = out / ARCHIVE_ROOT
    payload.mkdir()
    run(["ditto", str(app), str(payload / APP)])
    native = payload / NATIVE
    version_output = run([str(native), "--version"])
    if version_output not in ("MegaFloyd version " + version, "megafloyd version " + version):
        raise ValueError("bundled Go executable has the wrong release version: " + version_output)
    provenance = json.loads((payload / APP / "Contents/Resources/provenance.json").read_text())
    web_identity = json.loads((payload / WEB / "release.json").read_text())
    if provenance.get("version") != version or provenance.get("buildId") != build_id:
        raise ValueError("desktop release identity differs from the requested release")
    if web_identity.get("version") != version or web_identity.get("build_id") != build_id:
        raise ValueError("web release identity differs from the requested release")
    build_receipt_path = pathlib.Path(args.build_receipt).resolve()
    build_receipt = json.loads(build_receipt_path.read_text())
    receipt_version = build_receipt.get("version", "").removeprefix("MegaFloyd version ").removeprefix("megafloyd version ")
    receipt_hashes = [build_receipt.get("sha256")] + [item.get("sha256") for item in build_receipt.get("artifacts", [])]
    if (build_receipt.get("build_id") != build_id or receipt_version != version or
            sha256(native) not in receipt_hashes):
        raise ValueError("Go build receipt does not match the executable and requested release")
    run(["codesign", "--verify", "--deep", "--strict", str(payload / APP)])
    (payload / "bin").mkdir()
    launcher = payload / "bin/megafloyd"
    launcher.write_text(RELEASE_LAUNCHER.format(version=version, build_id=build_id, release_id=release_id))
    launcher.chmod(0o755)
    (payload / "bin/megafloid").symlink_to("megafloyd")
    shutil.copy2(root / "scripts/megafloyd-startup.py", payload / "bin/megafloyd-startup")
    (payload / "bin/megafloyd-startup").chmod(0o755)
    shutil.copy2(__file__, payload / "install.py")
    tree = inventory(payload)
    web_records = [item for item in tree if item["path"].startswith(WEB + "/")]
    desktop_records = [item for item in tree if item["path"].startswith(APP + "/")]
    identity = {"schema_version": 1, "product": PRODUCT, "version": version,
                "release_id": release_id, "build_id": build_id, "built_at": timestamp(),
                "target": target, "source": source_receipt(root), "signing": provenance.get("signing"),
                "compatibility": {"web_protocol_major": 2, "go_store": "<MEGAFLOYD_HOME>/data/workspaces/<sha256-canonical-cwd>/floyd.db",
                                  "explicit_data_directory": "per-account recorded mapping or explicit opt-in override",
                                  "activation": "new-processes-only", "automatic_database_rollback": False},
                "components": {"go": {"path": NATIVE, "sha256": sha256(native), "version": version,
                                       "build_receipt_sha256": sha256(build_receipt_path),
                                       "toolchain": run([args.go, "version"])},
                               "web": {"path": WEB, "inventory_sha256": digest(web_records), "entries": len(web_records)},
                               "desktop": {"path": APP, "bundle_id": "ai.legacy.megafloyd",
                                           "inventory_sha256": digest(desktop_records)},
                               "inventory_sha256": digest(tree)}}
    release = dict(identity, inventory=tree)
    (payload / "release.json").write_bytes(encoded(release))
    archive_name = "megafloyd-" + target + ".tar.gz"
    archive = out / archive_name
    with tarfile.open(archive, "w:gz", compresslevel=6) as tar:
        entries = [payload] + [payload / item["path"] for item in tree] + [payload / "release.json"]
        for path in entries:
            name = ARCHIVE_ROOT + ("/" + path.relative_to(payload).as_posix() if path != payload else "")
            info = tar.gettarinfo(str(path), arcname=name)
            info.uid = info.gid = 0
            info.uname = info.gname = ""
            if path.is_file() and not path.is_symlink():
                info.type = tarfile.REGTYPE
                info.linkname = ""
                info.size = path.stat().st_size
                with path.open("rb") as source:
                    tar.addfile(info, source)
            else:
                tar.addfile(info)
    manifest = dict(identity, release_sha256=sha256(payload / "release.json"),
                    platforms={target: {"filename": archive_name, "size": archive.stat().st_size,
                                        "sha256": sha256(archive), "unpacked_bytes": sum(i.get("size", 0) for i in tree)}})
    manifest_path = out / "manifest.json"
    manifest_path.write_bytes(encoded(manifest))
    latest = {"schema_version": 1, "product": PRODUCT, "version": version,
              "release_id": release_id, "manifest": "releases/" + release_id + "/manifest.json",
              "manifest_sha256": sha256(manifest_path), "published_at": None}
    (out / "latest.json").write_bytes(encoded(latest))
    installer = out / "install.sh"
    installer.write_text("#!/bin/sh\nset -eu\ncommand -v python3 >/dev/null 2>&1 || { echo 'MegaFloyd installer requires python3' >&2; exit 1; }\nexec python3 - install \"$@\" <<'MEGAFLOYD_INSTALLER_PY'\n" + pathlib.Path(__file__).read_text() + "\nMEGAFLOYD_INSTALLER_PY\n")
    installer.chmod(0o755)
    (out / "SHA256SUMS").write_text("".join(sha256(out / name) + "  " + name + "\n" for name in [archive_name, "manifest.json", "install.sh"]))
    print(json.dumps({"manifest": str(manifest_path), "manifest_sha256": sha256(manifest_path),
                      "archive": str(archive), "archive_sha256": sha256(archive),
                      "archive_bytes": archive.stat().st_size, "release_id": release_id,
                      "inventory_entries": len(tree)}, indent=2))


def fetch(url, destination, maximum):
    if not url.startswith("https://"):
        raise ValueError("release download must use HTTPS")
    subprocess.run(["curl", "--fail", "--silent", "--show-error", "--location", "--proto", "=https",
                    "--proto-redir", "=https", "--connect-timeout", "10", "--max-time", "180",
                    "--max-filesize", str(maximum), "--output", str(destination), url],
                   check=True, timeout=185)
    if destination.stat().st_size > maximum:
        raise ValueError("download exceeded its declared size")


def check_identity(manifest):
    if manifest.get("schema_version") != 1 or manifest.get("product") != PRODUCT:
        raise ValueError("manifest is not a MegaFloyd release")
    for name in ["version", "build_id", "release_id"]:
        safe_name(manifest[name])
    if manifest["release_id"] != manifest["version"] + "-" + manifest["build_id"]:
        raise ValueError("manifest release identity is inconsistent")


def extract(archive, destination, byte_limit):
    with tarfile.open(archive, "r:gz") as tar:
        members = tar.getmembers()
        seen = set()
        total = 0
        for member in members:
            name = relative(member.name)
            if not name.parts or name.parts[0] != ARCHIVE_ROOT or member.name in seen:
                raise ValueError("archive has an invalid root or duplicate path")
            seen.add(member.name)
            if not (member.isfile() or member.isdir() or member.issym()):
                raise ValueError("archive contains an unsupported member type")
            total += member.size
            if total > byte_limit + 32 * 1024 * 1024:
                raise ValueError("archive exceeded its declared unpacked size")
        # Create directories and regular files before links. A link never becomes an extraction parent.
        for member in sorted(members, key=lambda item: (len(pathlib.PurePosixPath(item.name).parts), item.name)):
            path = destination / member.name
            inside(destination, path)
            if member.isdir():
                path.mkdir(parents=True, exist_ok=True)
                path.chmod(member.mode & 0o777)
            elif member.isfile():
                path.parent.mkdir(parents=True, exist_ok=True)
                with tar.extractfile(member) as source, path.open("xb") as output:
                    shutil.copyfileobj(source, output)
                path.chmod(member.mode & 0o777)
        for member in members:
            if member.issym():
                path = destination / member.name
                path.parent.mkdir(parents=True, exist_ok=True)
                if pathlib.PurePosixPath(member.linkname).is_absolute():
                    raise ValueError("absolute package symlink is not allowed")
                inside(destination / ARCHIVE_ROOT, path.parent / member.linkname)
                path.symlink_to(member.linkname)


def verify_tree(path, manifest):
    check_identity(manifest)
    release_file = path / "release.json"
    if sha256(release_file) != manifest["release_sha256"]:
        raise ValueError("release inventory checksum mismatch")
    release = json.loads(release_file.read_text())
    for key in ["version", "build_id", "release_id", "target"]:
        if release.get(key) != manifest.get(key):
            raise ValueError("package identity mismatch: " + key)
    actual = inventory(path)
    if actual != release["inventory"] or digest(actual) != manifest["components"]["inventory_sha256"]:
        raise ValueError("package files do not match the manifest inventory")
    result = run([str(path / "bin/megafloyd"), "--version"])
    if result not in ("MegaFloyd version " + manifest["version"], "megafloyd version " + manifest["version"]):
        raise ValueError("package version check failed: " + result)
    if sys.platform == "darwin":
        run(["codesign", "--verify", "--deep", "--strict", str(path / APP)])
    return release


def backup(path):
    if not path.exists() and not path.is_symlink():
        return None
    saved = path.with_name(path.name + "." + timestamp() + ".bak-release")
    if path.is_symlink():
        saved.symlink_to(os.readlink(path))
    elif path.is_file():
        shutil.copy2(path, saved)
    else:
        raise ValueError("existing installation entry is not a file or link: " + str(path))
    return saved


def replace_link(path, target):
    staged = path.with_name(path.name + ".staging-" + timestamp())
    staged.symlink_to(target)
    os.replace(staged, path)


def ensure_command_alias(prefix, name, target):
    alias = prefix / "bin" / name
    if alias.exists() or alias.is_symlink():
        owned = alias.is_symlink() and os.readlink(alias) == target
        return {"path": str(alias), "target": target, "created": False, "status": "available" if owned else "existing-entry-preserved"}
    alias.symlink_to(target)
    return {"path": str(alias), "target": target, "created": True, "status": "available"}


def ensure_spelling_alias(prefix):
    return ensure_command_alias(prefix, "megafloid", "megafloyd")


def register_desktop(prefix):
    """Keep macOS reopening this prefix's current app, without touching other installs."""
    if sys.platform != "darwin":
        return {"performed": False, "reason": "not macOS"}
    tool = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
    releases = (prefix / "releases").resolve()
    current = (prefix / APP).resolve(strict=True)
    inside(releases, current)
    identifier = "ai.legacy.megafloyd"
    with (current / "Contents/Info.plist").open("rb") as stream:
        if plistlib.load(stream).get("CFBundleIdentifier") != identifier:
            raise ValueError("current desktop bundle identity is not MegaFloyd")
    retired = []
    already_absent = []
    for candidate in sorted(releases.glob("*/" + APP)):
        resolved = candidate.resolve()
        if resolved == current:
            continue
        # An arbitrary symlink or another product under this prefix is not ours to unregister.
        if candidate.is_symlink() or releases not in resolved.parents:
            continue
        try:
            with (candidate / "Contents/Info.plist").open("rb") as stream:
                own = plistlib.load(stream).get("CFBundleIdentifier") == identifier
        except (OSError, plistlib.InvalidFileException):
            continue
        if own:
            try:
                run([tool, "-u", str(candidate)])
            except subprocess.CalledProcessError as error:
                # lsregister reports kLSApplicationNotFoundErr for an already removed entry.
                # The app's files and bundle identity were validated above; retirement is done.
                output = (error.stdout or "") + (error.stderr or "")
                if error.returncode != 1 or f"failed to scan {candidate}: -10814" not in output:
                    raise
                already_absent.append(str(candidate))
            else:
                retired.append(str(candidate))
    run([tool, "-f", str(prefix / APP)])
    return {"performed": True, "canonical_app": str(prefix / APP), "resolved_app": str(current),
            "unregistered_archived_apps": retired, "already_unregistered_archived_apps": already_absent,
            "files_removed": False}


def install(args):
    if not args.prefix:
        print(DESTINATION_ERROR, file=sys.stderr)
        return 2
    raw_prefix = pathlib.Path(args.prefix).expanduser()
    if not raw_prefix.is_absolute() or raw_prefix == pathlib.Path("/"):
        raise ValueError("installation prefix must be an explicit absolute user directory")
    prefix = raw_prefix.resolve()
    prefix.mkdir(parents=True, exist_ok=True)
    with (prefix / ".megafloyd-install.lock").open("a+") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError("another MegaFloyd installation is already running") from None
        with tempfile.TemporaryDirectory(prefix=".megafloyd-stage-", dir=prefix) as temporary:
            stage = pathlib.Path(temporary)
            if args.manifest:
                manifest_path = pathlib.Path(args.manifest).resolve()
            else:
                base = args.cdn.rstrip("/")
                latest_path = stage / "latest.json"
                fetch(base + "/latest.json", latest_path, 1024 * 1024)
                latest = json.loads(latest_path.read_text())
                if latest.get("product") != PRODUCT:
                    raise ValueError("latest release points to a different product")
                manifest_rel = str(relative(latest["manifest"]))
                if manifest_rel != "releases/" + safe_name(latest["release_id"]) + "/manifest.json":
                    raise ValueError("latest manifest path does not match its release")
                manifest_path = stage / "manifest.json"
                fetch(base + "/" + manifest_rel, manifest_path, 4 * 1024 * 1024)
                if sha256(manifest_path) != latest["manifest_sha256"]:
                    raise ValueError("manifest checksum mismatch")
            manifest = json.loads(manifest_path.read_text())
            check_identity(manifest)
            target = host_target()
            artifact = manifest.get("platforms", {}).get(target)
            if artifact is None or manifest.get("target") != target:
                raise ValueError("release does not contain this platform: " + target)
            if args.check:
                current = prefix / "current/release.json"
                installed = json.loads(current.read_text())["release_id"] if current.exists() else None
                print(json.dumps({"available": manifest["release_id"], "installed": installed,
                                  "update_available": installed != manifest["release_id"]}, indent=2))
                return 0
            if args.archive:
                archive = pathlib.Path(args.archive).resolve()
            elif args.manifest:
                archive = manifest_path.parent / str(relative(artifact["filename"]))
            else:
                archive = stage / "package.tar.gz"
                fetch(args.cdn.rstrip("/") + "/releases/" + manifest["release_id"] + "/" +
                      str(relative(artifact["filename"])), archive, artifact["size"])
            if archive.stat().st_size != artifact["size"] or sha256(archive) != artifact["sha256"]:
                raise ValueError("package checksum or size mismatch; installed release is unchanged")
            unpack = stage / "unpack"
            unpack.mkdir()
            extract(archive, unpack, artifact["unpacked_bytes"])
            payload = unpack / ARCHIVE_ROOT
            release = verify_tree(payload, manifest)
            if args.verify_only:
                print(json.dumps({"verified": True, "release_id": release["release_id"],
                                  "archive_sha256": artifact["sha256"]}, indent=2))
                return 0
            releases = prefix / "releases"
            releases.mkdir(exist_ok=True)
            inside(prefix, releases)
            installed = releases / manifest["release_id"]
            if installed.exists():
                verify_tree(installed, manifest)
            else:
                payload.rename(installed)
            current = prefix / "current"
            previous = os.readlink(current) if current.is_symlink() else None
            saved_current = backup(current)
            launcher = prefix / "bin/megafloyd"
            launcher.parent.mkdir(exist_ok=True)
            inside(prefix, launcher.parent)
            launcher_saved = backup(launcher)
            desktop = prefix / APP
            desktop_saved = backup(desktop)
            receipt_path = prefix / "install-receipt.json"
            receipt_saved = backup(receipt_path)
            created_aliases = []
            registration_attempted = False
            try:
                staged_launcher = launcher.with_name(".megafloyd-" + timestamp())
                staged_launcher.write_text(PREFIX_LAUNCHER)
                staged_launcher.chmod(0o755)
                os.replace(staged_launcher, launcher)
                replace_link(desktop, "current/" + APP)
                replace_link(current, "releases/" + manifest["release_id"])
                result = run([str(launcher), "--version"])
                if result not in ("MegaFloyd version " + manifest["version"], "megafloyd version " + manifest["version"]):
                    raise ValueError("activated launcher version differs from the release")
                spelling_alias = ensure_spelling_alias(prefix)
                created_aliases.append(spelling_alias)
                startup_alias = ensure_command_alias(prefix, "megafloyd-startup", "../current/bin/megafloyd-startup")
                created_aliases.append(startup_alias)
                registration_attempted = True
                desktop_registration = register_desktop(prefix)
                receipt = {"desktop_registration": desktop_registration, "spelling_alias": spelling_alias, "startup_alias": startup_alias, "product": PRODUCT, "version": manifest["version"],
                           "build_id": manifest["build_id"], "release_id": manifest["release_id"],
                           "prefix": str(prefix), "current": str(installed), "previous": previous,
                           "archive_sha256": artifact["sha256"], "manifest_sha256": sha256(manifest_path),
                           "installed_at": timestamp(), "runtime_restarted": False,
                           "backups": [str(p) for p in [saved_current, launcher_saved, desktop_saved] if p]}
                staged_receipt = stage / "install-receipt.json"
                staged_receipt.write_bytes(encoded(receipt))
                os.replace(staged_receipt, receipt_path)
            except BaseException:
                for record in created_aliases:
                    if record["created"]:
                        alias = pathlib.Path(record["path"])
                        if alias.is_symlink() and os.readlink(alias) == record["target"]: alias.unlink()
                for path, saved in [(current, saved_current), (launcher, launcher_saved),
                                    (desktop, desktop_saved), (receipt_path, receipt_saved)]:
                    if path.exists() or path.is_symlink():
                        if path.is_dir() and not path.is_symlink():
                            raise RuntimeError("unexpected directory during rollback: " + str(path))
                        path.unlink()
                    if saved:
                        saved.rename(path)
                if registration_attempted and desktop.exists():
                    register_desktop(prefix)
                raise
            print(json.dumps(receipt, indent=2))
            print("Complete release installed. Existing processes continue using their current release; restart the identified idle MegaFloyd runtime to activate it.")
            return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    pack = sub.add_parser("package", help="package an already built and verified Mac application")
    for name in ["repo", "app", "out", "version", "build-id", "build-receipt", "go"]:
        pack.add_argument("--" + name, required=True)
    setup = sub.add_parser("install", help="install one complete release under an explicit prefix")
    setup.add_argument("--prefix")
    setup.add_argument("--manifest", help="local manifest for an offline install")
    setup.add_argument("--archive", help="local archive; defaults beside a local manifest")
    setup.add_argument("--cdn", default=CDN)
    setup.add_argument("--check", action="store_true")
    setup.add_argument("--verify-only", action="store_true")
    args = parser.parse_args()
    try:
        return package(args) if args.command == "package" else install(args)
    except (ValueError, KeyError, OSError, subprocess.SubprocessError, tarfile.TarError) as error:
        print("MegaFloyd release failed: " + str(error), file=sys.stderr)
        if isinstance(error, subprocess.CalledProcessError) and error.stderr:
            print(error.stderr, file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
