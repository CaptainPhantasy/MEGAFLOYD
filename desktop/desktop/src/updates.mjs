import { execFile } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { promisify } from 'node:util';

const execute = promisify(execFile);

export function installedPackage(resourcesPath) {
  const release = path.resolve(fs.realpathSync(resourcesPath), '../../..');
  const metadata = JSON.parse(fs.readFileSync(path.join(release, 'release.json'), 'utf8'));
  if (metadata.product !== 'megafloyd' || !metadata.release_id ||
      path.basename(path.dirname(release)) !== 'releases' || path.basename(release) !== metadata.release_id) {
    throw new Error('This application is not an installed MegaFloyd release.');
  }
  const prefix = path.resolve(release, '../..');
  const launcher = path.join(prefix, 'bin', 'megafloyd');
  const current = JSON.parse(fs.readFileSync(path.join(prefix, 'current', 'release.json'), 'utf8'));
  if (current.product !== 'megafloyd') throw new Error('The installed release belongs to another application.');
  fs.accessSync(launcher, fs.constants.X_OK);
  return { prefix, launcher, releaseId: metadata.release_id };
}

export async function checkPackageUpdate(resourcesPath) {
  const installation = installedPackage(resourcesPath);
  const { stdout } = await execute(installation.launcher, ['update', '--check'], {
    timeout: 200_000,
    maxBuffer: 1024 * 1024,
  });
  const result = JSON.parse(stdout);
  if (typeof result.update_available !== 'boolean' || typeof result.available !== 'string') {
    throw new Error('The MegaFloyd release service returned an invalid update result.');
  }
  return result;
}

export async function installPackageUpdate(resourcesPath) {
  const installation = installedPackage(resourcesPath);
  await execute(installation.launcher, ['update'], {
    timeout: 300_000,
    maxBuffer: 4 * 1024 * 1024,
  });
}
