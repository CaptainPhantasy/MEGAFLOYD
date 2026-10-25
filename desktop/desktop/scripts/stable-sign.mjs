import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

export function loadSigningIdentity() {
  const file = process.env.MEGAFLOYD_SIGNING_METADATA || path.join(os.homedir(), '.megafloyd', 'signing', 'identity.json');
  const identity = JSON.parse(fs.readFileSync(file, 'utf8'));
  if (!/^[0-9a-f]{40}$/i.test(identity.certificate_sha1) || !path.isAbsolute(identity.keychain) || !path.isAbsolute(identity.password_file)) throw new Error('MEGAFLOYD signing metadata is invalid');
  const password = fs.readFileSync(identity.password_file, 'utf8');
  try {
    execFileSync('security', ['unlock-keychain', '-p', password, identity.keychain], { stdio: ['ignore', 'pipe', 'pipe'] });
  } catch {
    throw new Error('Could not unlock the dedicated MEGAFLOYD signing keychain');
  }
  return identity;
}

export function signCode(file, identifier, identity = loadSigningIdentity()) {
  execFileSync('codesign', ['--force', '--sign', identity.certificate_sha1, '--keychain', identity.keychain, '--identifier', identifier, '--timestamp=none', file], { stdio: 'inherit' });
  execFileSync('codesign', ['--verify', '--strict', '-R', `=identifier "${identifier}" and certificate root = H"${identity.certificate_sha1}"`, file], { stdio: 'inherit' });
}

export function signApp(appPath, identity = loadSigningIdentity()) {
  const native = path.join(appPath, 'Contents', 'Resources', 'megafloyd', 'bin', 'megafloyd');
  execFileSync('codesign', ['--verify', '--strict', '-R', `=identifier "ai.legacy.megafloyd.engine" and certificate root = H"${identity.certificate_sha1}"`, native], { stdio: 'inherit' });
  execFileSync('codesign', ['--force', '--deep', '--preserve-metadata=identifier,entitlements,flags', '--sign', identity.certificate_sha1, '--keychain', identity.keychain, '--timestamp=none', appPath], { stdio: 'inherit' });
  execFileSync('codesign', ['--verify', '--deep', '--strict', '-R', `=identifier "ai.legacy.megafloyd" and certificate root = H"${identity.certificate_sha1}"`, appPath], { stdio: 'inherit' });
  return { certificate_sha1: identity.certificate_sha1, kind: 'persistent-local-certificate', native_identifier: 'ai.legacy.megafloyd.engine', desktop_identifier: 'ai.legacy.megafloyd' };
}
