import { readdir, realpath, stat } from 'node:fs/promises';
import { homedir } from 'node:os';
import { dirname, isAbsolute, join } from 'node:path';
import type { FsBrowseEntry, FsBrowseResponse } from '@legacy-ai/agent-core-v2/app/hostFolderBrowser/hostFolderBrowser';
import { GoRequestError } from './client';

export async function browseHostFolders(path?: string): Promise<FsBrowseResponse> {
  const target = path ?? homedir();
  if (!isAbsolute(target) || target.includes('\0')) throw new GoRequestError(400, 'Choose an absolute folder path');
  try {
    const current = await realpath(target);
    const children = await readdir(current, { withFileTypes: true });
    const entries: FsBrowseEntry[] = [];
    for (const child of children) {
      const childPath = join(current, child.name);
      let directory = child.isDirectory();
      if (child.isSymbolicLink()) {
        try { directory = (await stat(childPath)).isDirectory(); }
        catch (error) {
          const code = (error as NodeJS.ErrnoException).code || '';
          if (['ENOENT', 'ENOTDIR'].includes(code)) continue;
          if (code === 'EACCES' || code === 'EPERM') { entries.push({ name: `${child.name} (access denied)`, path: childPath, is_dir: true }); continue; }
          throw hostFolderError(error, childPath);
        }
      }
      if (directory) entries.push({ name: child.name, path: childPath, is_dir: true });
    }
    entries.sort((a, b) => Number(a.name.startsWith('.')) - Number(b.name.startsWith('.')) || a.name.localeCompare(b.name));
    if (current === await realpath(homedir()) && current !== '/') {
      entries.unshift({ name: 'Filesystem', path: '/', is_dir: true });
      if (process.platform === 'darwin') entries.splice(1, 0, { name: 'Mounted volumes', path: '/Volumes', is_dir: true });
    }
    const parent = dirname(current);
    return { path: current, parent: parent === current ? null : parent, entries };
  } catch (error) { throw hostFolderError(error, target); }
}

function hostFolderError(error: unknown, path: string): Error {
  if (error instanceof GoRequestError) return error;
  const code = (error as NodeJS.ErrnoException).code;
  if (code === 'ENOENT' || code === 'ENOTDIR') return new GoRequestError(404, `Folder not found: ${path}`);
  if (code === 'EACCES' || code === 'EPERM') {
    const help = process.platform === 'darwin' ? ' Check the folder’s account permissions and MEGAFLOYD’s Files and Folders access in System Settings > Privacy & Security.' : ' Check the folder’s account permissions.';
    return new GoRequestError(403, `The operating system denied access to ${path}.${help}`);
  }
  return error instanceof Error ? error : new Error(String(error));
}
