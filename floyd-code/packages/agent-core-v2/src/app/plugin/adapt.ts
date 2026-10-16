import { mkdir, readFile, readdir, stat, writeFile } from 'node:fs/promises';
import path from 'node:path';

import { parse as parseToml } from 'smol-toml';

import { HookDefSchema, type HookDefConfig } from '#/features/externalHooks/configSection';
import { HOOK_EVENT_TYPES } from '#/features/externalHooks/internal/types';
import { McpServerConfigSchema, type McpServerConfig } from '#/mcpCore/config-schema';

import { PLUGIN_NAME_REGEX, type PluginDiagnostic, type PluginInstallSkip } from './types';

const CLAUDE_PLUGIN_PATHS = ['.claude-plugin/plugin.json', 'claude-plugin/plugin.json'];
const CLAUDE_PACK_PATHS = ['.claude-plugin/marketplace.json', 'claude-plugin/marketplace.json'];
const GEMINI_EXTENSION_PATH = 'gemini-extension.json';
const FLOYD_MANIFEST_NAME = 'floyd.plugin.json';
const COMMANDS_DIR = 'commands';
const SKILLS_DIR = 'skills';
const AGENTS_DIR = 'agents';
const ROOT_SKILL_FILE = 'SKILL.md';
const FALLBACK_PLUGIN_NAME = 'imported-plugin';

export type ForeignPluginKind = 'claude-plugin' | 'claude-pack' | 'gemini-extension';

export interface ForeignPlugin {
  readonly kind: ForeignPluginKind;
  readonly path: string;
}

export interface SynthOutcome {
  readonly diagnostics: readonly PluginDiagnostic[];
}

export interface PackEntry {
  readonly name: string;
  readonly dir: string;
  readonly description?: string;
  readonly version?: string;
}

export interface PackSelection {
  readonly entries: readonly PackEntry[];
  readonly skipped: readonly PluginInstallSkip[];
}

export async function detectForeignPlugin(root: string): Promise<ForeignPlugin | undefined> {
  for (const rel of CLAUDE_PLUGIN_PATHS) {
    if (await isFile(path.join(root, rel))) return { kind: 'claude-plugin', path: rel };
  }
  for (const rel of CLAUDE_PACK_PATHS) {
    if (await isFile(path.join(root, rel))) return { kind: 'claude-pack', path: rel };
  }
  if (await isFile(path.join(root, GEMINI_EXTENSION_PATH))) {
    return { kind: 'gemini-extension', path: GEMINI_EXTENSION_PATH };
  }
  return undefined;
}

export async function hasRecognizablePluginShape(dir: string): Promise<boolean> {
  if ((await detectForeignPlugin(dir)) !== undefined) return true;
  return (
    (await isDir(path.join(dir, SKILLS_DIR))) ||
    (await isDir(path.join(dir, COMMANDS_DIR))) ||
    (await isDir(path.join(dir, AGENTS_DIR))) ||
    (await isFile(path.join(dir, ROOT_SKILL_FILE)))
  );
}

const SWEEP_MAX_DIRS = 500;
const IGNORED_FILE_NAMES = new Set([
  'readme.md',
  'license.md',
  'contributing.md',
  'changelog.md',
  'code_of_conduct.md',
  'security.md',
]);

export interface LoosePieces {
  readonly skills: readonly string[];
  readonly commands: readonly string[];
}

export async function collectLoosePieces(root: string): Promise<LoosePieces | undefined> {
  const skills = new Set<string>();
  const commands = new Set<string>();
  let scanned = 0;
  const walk = async (dir: string, depth: number): Promise<void> => {
    if (scanned >= SWEEP_MAX_DIRS || depth > 5) return;
    scanned += 1;
    let entries;
    try {
      entries = await readdir(dir, { withFileTypes: true });
    } catch {
      return;
    }
    const rel = (p: string) => `./${path.relative(root, p).split(path.sep).join('/')}`;
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (entry.name === COMMANDS_DIR) {
          const markdowns = await listMarkdownFiles(full);
          for (const file of markdowns) commands.add(rel(file));
          continue;
        }
        if (await isFile(path.join(full, ROOT_SKILL_FILE))) {
          skills.add(rel(full));
          continue;
        }
        await walk(full, depth + 1);
      } else if (
        depth === 0 &&
        entry.isFile() &&
        entry.name.toLowerCase().endsWith('.md') &&
        !IGNORED_FILE_NAMES.has(entry.name.toLowerCase())
      ) {
        commands.add(rel(full));
      }
    }
  };
  await walk(root, 0);
  if (skills.size === 0 && commands.size === 0) return undefined;
  return { skills: [...skills], commands: [...commands] };
}

async function listMarkdownFiles(dir: string): Promise<readonly string[]> {
  const out: string[] = [];
  const entries = await readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...(await listMarkdownFiles(full)));
    } else if (entry.isFile() && entry.name.endsWith('.md')) {
      out.push(full);
    }
  }
  return out;
}

export async function synthesizeFromLoosePieces(
  root: string,
  pieces: LoosePieces,
): Promise<SynthOutcome> {
  const manifest: Record<string, unknown> = {
    name: sanitizePluginName(path.basename(root)),
    skills: pieces.skills.length > 0 ? pieces.skills : undefined,
    commands: pieces.commands.length > 0 ? pieces.commands : undefined,
  };
  return writeManifest(root, manifest, []);
}

export async function synthesizeFromClaudePlugin(
  root: string,
  foreignPath: string,
): Promise<SynthOutcome> {
  const tag = await readJsonObject(path.join(root, foreignPath));
  const diagnostics: PluginDiagnostic[] = [];
  const name = sanitizePluginName(stringField(tag, 'name') ?? '');
  const author = readClaudeAuthor(tag['author']);
  const repository = typeof tag['repository'] === 'string' ? stringField(tag, 'repository') : undefined;
  const website = stringField(tag, 'homepage') ?? repository ?? author?.url;
  const manifest: Record<string, unknown> = {
    name,
    version: stringField(tag, 'version'),
    description: stringField(tag, 'description'),
    keywords: stringArrayField(tag, 'keywords'),
    homepage: website,
    license: stringField(tag, 'license'),
    author: author === undefined ? undefined : { name: author.name, email: author.email },
    skills: relativePathListField(tag['skills']),
    agents: relativePathListField(tag['agents']),
    commands: relativePathListField(tag['commands']),
    hooks: await readClaudeHooks(root, tag['hooks'], diagnostics),
    mcpServers: await readMcpServerMap(root, tag['mcpServers'], diagnostics),
    interface: {
      displayName: stringField(tag, 'name'),
      shortDescription: truncate(stringField(tag, 'description'), 100),
      developerName: author?.name,
      websiteURL: website,
    },
  };
  noteUnsupportedKeys(tag, ['name', 'description', 'version', 'author', 'homepage', 'repository', 'license', 'keywords', 'skills', 'agents', 'commands', 'hooks', 'mcpServers'], diagnostics);
  return writeManifest(root, manifest, diagnostics);
}

export async function synthesizeFromGeminiExtension(
  root: string,
  foreignPath: string,
): Promise<SynthOutcome> {
  const tag = await readJsonObject(path.join(root, foreignPath));
  const diagnostics: PluginDiagnostic[] = [];
  const contextFileName = stringField(tag, 'contextFileName');
  const contextPath = contextFileName ?? 'GEMINI.md';
  const contextExists = await isFile(path.join(root, contextPath));
  if (tag['excludeTools'] !== undefined) {
    diagnostics.push({
      severity: 'info',
      message: '"excludeTools" is present but not used by Floyd (adapted from Gemini CLI)',
    });
  }
  const commands = await convertGeminiCommands(root, diagnostics);
  const manifest: Record<string, unknown> = {
    name: sanitizePluginName(stringField(tag, 'name') ?? ''),
    version: stringField(tag, 'version'),
    mcpServers: await readMcpServerMap(root, tag['mcpServers'], diagnostics),
    commands: commands.length > 0 ? commands : undefined,
    systemPromptPath: contextExists ? `./${contextPath}` : undefined,
    interface: {
      displayName: stringField(tag, 'name'),
      developerName: undefined,
    },
  };
  return writeManifest(root, manifest, diagnostics);
}

export async function selectClaudePackEntries(
  root: string,
  foreignPath: string,
  pick: string | undefined,
): Promise<PackSelection> {
  const pack = await readJsonObject(path.join(root, foreignPath));
  const rawEntries = Array.isArray(pack['plugins']) ? pack['plugins'] : [];
  const skipped: PluginInstallSkip[] = [];
  const entries: PackEntry[] = [];
  for (const raw of rawEntries) {
    if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) continue;
    const tag = raw as Record<string, unknown>;
    const name = stringField(tag, 'name');
    if (name === undefined) continue;
    const source = tag['source'];
    if (typeof source === 'string' && source.startsWith('./')) {
      const dir = path.resolve(root, source);
      if (!isWithin(dir, root) || !(await isDir(dir))) {
        skipped.push({ name, reason: `pack entry folder is missing (${source})` });
        continue;
      }
      entries.push({
        name,
        dir,
        description: stringField(tag, 'description'),
        version: stringField(tag, 'version'),
      });
      continue;
    }
    if (typeof source === 'object' && source !== null && !Array.isArray(source)) {
      const repo = stringField(source as Record<string, unknown>, 'repo');
      if (repo !== undefined) {
        skipped.push({
          name,
          reason: 'pack entry points at another repository',
          installCommand: `/plugins install https://github.com/${repo}`,
        });
        continue;
      }
    }
    skipped.push({ name, reason: 'pack entry source is not a folder inside the pack' });
  }
  if (pick !== undefined) {
    const exact = entries.find((entry) => entry.name === pick);
    const loose = exact ?? entries.find((entry) => entry.name.toLowerCase() === pick.toLowerCase());
    if (loose === undefined) {
      throw new Error(
        `This pack has no plugin named "${pick}". Installed names: ${entries.map((entry) => entry.name).join(', ') || 'none'}.`,
      );
    }
    return { entries: [loose], skipped };
  }
  if (entries.length === 0) {
    const commands = skipped
      .map((skip) => skip.installCommand)
      .filter((command): command is string => command !== undefined);
    throw new Error(
      commands.length > 0
        ? `This pack only points at other repositories. Install them directly: ${commands.join(' ')}`
        : 'This pack lists no installable plugins.',
    );
  }
  return { entries, skipped };
}

export async function synthesizeFromPackEntry(
  root: string,
  entry: PackEntry,
): Promise<SynthOutcome> {
  const inner = await detectForeignPlugin(entry.dir);
  if (inner?.kind === 'claude-plugin') {
    return synthesizeFromClaudePlugin(entry.dir, inner.path);
  }
  return synthesizeFromFolders(entry.dir, {
    name: entry.name,
    description: entry.description,
    version: entry.version,
  });
}

export async function synthesizeFromFolders(
  root: string,
  seed: { readonly name?: string; readonly description?: string; readonly version?: string },
): Promise<SynthOutcome> {  const diagnostics: PluginDiagnostic[] = [];
  const hasSkills = await isDir(path.join(root, SKILLS_DIR));
  const hasCommands = await isDir(path.join(root, COMMANDS_DIR));
  const hasAgents = await isDir(path.join(root, AGENTS_DIR));
  const rootSkill = await isFile(path.join(root, ROOT_SKILL_FILE));
  if (!hasSkills && !hasCommands && !hasAgents && !rootSkill) {
    throw new Error('no skills, commands, or agents were found in this folder');
  }
  const frontmatter = rootSkill ? await readSkillFrontmatter(path.join(root, ROOT_SKILL_FILE)) : {};
  const name = sanitizePluginName(
    seed.name ?? frontmatter['name'] ?? path.basename(root),
  );
  const manifest: Record<string, unknown> = {
    name,
    version: seed.version,
    description: seed.description ?? frontmatter['description'],
    skills: hasSkills ? [`./${SKILLS_DIR}`] : undefined,
    commands: hasCommands ? [`./${COMMANDS_DIR}`] : undefined,
    agents: hasAgents ? [`./${AGENTS_DIR}`] : undefined,
    interface: {
      displayName: seed.name ?? name,
      shortDescription: truncate(seed.description ?? frontmatter['description'], 100),
    },
  };
  return writeManifest(root, manifest, diagnostics);
}

async function writeManifest(
  root: string,
  manifest: Record<string, unknown>,
  diagnostics: PluginDiagnostic[],
): Promise<SynthOutcome> {
  const clean = Object.fromEntries(
    Object.entries(manifest).filter(([, value]) => value !== undefined),
  );
  await writeFile(path.join(root, FLOYD_MANIFEST_NAME), `${JSON.stringify(clean, null, 2)}\n`);
  return { diagnostics };
}

export async function setPluginNameInManifest(root: string, name: string): Promise<void> {
  const file = path.join(root, FLOYD_MANIFEST_NAME);
  const manifest = await readJsonObject(file);
  manifest['name'] = name;
  await writeFile(file, `${JSON.stringify(manifest, null, 2)}\n`);
}

function claudeHooksToFlat(
  raw: unknown,
  sourceLabel: string,
  diagnostics: PluginDiagnostic[],
): readonly HookDefConfig[] | undefined {
  if (raw === undefined) return undefined;
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
    diagnostics.push({ severity: 'warn', message: `"hooks" must be an object (${sourceLabel})` });
    return undefined;
  }
  const out: HookDefConfig[] = [];
  for (const [event, handlers] of Object.entries(raw as Record<string, unknown>)) {
    if (!Array.isArray(handlers)) continue;
    if (!(HOOK_EVENT_TYPES as readonly string[]).includes(event)) {
      diagnostics.push({
        severity: 'warn',
        message: `hook event "${event}" is not supported by Floyd and was skipped`,
      });
      continue;
    }
    for (const handler of handlers) {
      if (typeof handler !== 'object' || handler === null || Array.isArray(handler)) continue;
      const tag = handler as Record<string, unknown>;
      const hooks = Array.isArray(tag['hooks']) ? tag['hooks'] : [handler];
      for (const hook of hooks) {
        if (typeof hook !== 'object' || hook === null || Array.isArray(hook)) continue;
        const entry = hook as Record<string, unknown>;
        const hookType = entry['type'];
        if (hookType !== undefined && hookType !== 'command') {
          diagnostics.push({
            severity: 'warn',
            message: `hook type "${typeof hookType === 'string' ? hookType : typeof hookType}" is not supported by Floyd and was skipped`,
          });
          continue;
        }
        const candidate = {
          event,
          matcher: typeof tag['matcher'] === 'string' ? tag['matcher'] : undefined,
          command: entry['command'],
          timeout: typeof entry['timeout'] === 'number' ? entry['timeout'] : undefined,
        };
        const parsed = HookDefSchema.safeParse(candidate);
        if (parsed.success) {
          out.push(parsed.data);
        } else {
          diagnostics.push({
            severity: 'warn',
            message: `a "${event}" hook was skipped: ${firstIssue(parsed.error.message)}`,
          });
        }
      }
    }
  }
  return out.length > 0 ? out : undefined;
}

async function readClaudeHooks(
  root: string,
  raw: unknown,
  diagnostics: PluginDiagnostic[],
): Promise<readonly HookDefConfig[] | undefined> {
  if (raw === undefined) return undefined;
  let value = raw;
  if (typeof raw === 'string') {
    value = await readJsonObject(path.join(root, raw));
  }
  return claudeHooksToFlat(value, 'adapted from Claude Code', diagnostics);
}

async function readMcpServerMap(
  root: string,
  raw: unknown,
  diagnostics: PluginDiagnostic[],
): Promise<Record<string, McpServerConfig> | undefined> {
  if (raw === undefined) return undefined;
  let value = raw;
  if (typeof raw === 'string') {
    value = await readJsonObject(path.join(root, raw));
  }
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    diagnostics.push({ severity: 'warn', message: '"mcpServers" must be an object and was skipped' });
    return undefined;
  }
  const out: Record<string, McpServerConfig> = {};
  for (const [name, entry] of Object.entries(value as Record<string, unknown>)) {
    const normalized = normalizeMcpEntry(entry);
    const parsed = McpServerConfigSchema.safeParse(normalized);
    if (parsed.success) {
      out[name] = parsed.data;
    } else {
      diagnostics.push({
        severity: 'warn',
        message: `MCP server "${name}" was skipped: ${firstIssue(parsed.error.message)}`,
      });
    }
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

function normalizeMcpEntry(entry: unknown): unknown {
  if (typeof entry !== 'object' || entry === null || Array.isArray(entry)) return entry;
  const tag = entry as Record<string, unknown>;
  if (typeof tag['command'] === 'string' && tag['args'] === undefined) {
    const parts = tag['command'].trim().split(/\s+/);
    if (parts.length > 1) {
      return { ...tag, command: parts[0], args: parts.slice(1) };
    }
  }
  return entry;
}

async function convertGeminiCommands(
  root: string,
  diagnostics: PluginDiagnostic[],
): Promise<readonly string[]> {
  const commandsDir = path.join(root, COMMANDS_DIR);
  if (!(await isDir(commandsDir))) return [];
  await mkdir(path.join(root, '.floyd-plugin'), { recursive: true });
  const outDir = path.join(root, '.floyd-plugin', 'commands');
  await mkdir(outDir, { recursive: true });
  const files = await listTomlFiles(commandsDir);
  const written: string[] = [];
  for (const file of files) {
    try {
      const parsed = parseToml(await readFile(file, 'utf8')) as Record<string, unknown>;
      const prompt = parsed['prompt'];
      if (typeof prompt !== 'string' || prompt.trim().length === 0) {
        diagnostics.push({
          severity: 'warn',
          message: `command file "${path.basename(file)}" has no prompt and was skipped`,
        });
        continue;
      }
      const description = typeof parsed['description'] === 'string' ? parsed['description'] : '';
      const body =
        description.length > 0
          ? `---\ndescription: ${description}\n---\n\n${prompt}\n`
          : `${prompt}\n`;
      const relative = path.relative(commandsDir, file).replace(/\.toml$/i, '.md');
      const target = path.join(outDir, relative);
      await mkdir(path.dirname(target), { recursive: true });
      await writeFile(target, body);
      written.push(`./.floyd-plugin/commands/${relative.split(path.sep).join('/')}`);
    } catch (error) {
      diagnostics.push({
        severity: 'warn',
        message: `command file "${path.basename(file)}" could not be read: ${(error as Error).message}`,
      });
    }
  }
  return written;
}

async function listTomlFiles(dir: string): Promise<readonly string[]> {
  const out: string[] = [];
  const entries = await readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...(await listTomlFiles(full)));
    } else if (entry.isFile() && entry.name.endsWith('.toml')) {
      out.push(full);
    }
  }
  return out.toSorted((a, b) => a.localeCompare(b));
}

function relativePathListField(raw: unknown): readonly string[] | undefined {
  if (raw === undefined) return undefined;
  const entries = typeof raw === 'string' ? [raw] : Array.isArray(raw) ? raw : [];
  const cleaned = entries
    .filter((entry): entry is string => typeof entry === 'string')
    .map((entry) => (entry.startsWith('./') || entry.startsWith('/') ? entry : `./${entry}`));
  return cleaned.length > 0 ? cleaned : undefined;
}

function readClaudeAuthor(
  raw: unknown,
): { name?: string; email?: string; url?: string } | undefined {
  if (typeof raw === 'string') return { name: raw };
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return undefined;
  const tag = raw as Record<string, unknown>;
  const name = stringField(tag, 'name');
  const email = stringField(tag, 'email');
  const url = stringField(tag, 'url');
  if (name === undefined && email === undefined && url === undefined) return undefined;
  return { name, email, url };
}

function noteUnsupportedKeys(
  tag: Record<string, unknown>,
  known: readonly string[],
  diagnostics: PluginDiagnostic[],
): void {
  for (const key of Object.keys(tag)) {
    if (known.includes(key)) continue;
    diagnostics.push({
      severity: 'info',
      message: `"${key}" is present but not used by Floyd (adapted from Claude Code)`,
    });
  }
}

async function readSkillFrontmatter(file: string): Promise<Record<string, string>> {
  const text = await readFile(file, 'utf8');
  const match = /^---\r?\n([\s\S]*?)\r?\n---/.exec(text);
  if (match === null) return {};
  const out: Record<string, string> = {};
  for (const line of match[1]!.split(/\r?\n/)) {
    const pair = /^(name|description):\s*(.+)$/.exec(line.trim());
    if (pair !== null) out[pair[1]!] = pair[2]!.trim();
  }
  return out;
}

export function sanitizePluginName(input: string): string {
  const cleaned = input
    .toLowerCase()
    .replaceAll(/[^a-z0-9_-]+/g, '-')
    .replaceAll(/-{2,}/g, '-')
    .replace(/^[^a-z0-9]+/, '')
    .replace(/-$/, '')
    .slice(0, 64);
  return PLUGIN_NAME_REGEX.test(cleaned) ? cleaned : FALLBACK_PLUGIN_NAME;
}

function truncate(value: string | undefined, max: number): string | undefined {
  if (value === undefined) return undefined;
  return value.length > max ? `${value.slice(0, max - 1)}…` : value;
}

function firstIssue(message: string): string {
  return message.split('\n')[0] ?? message;
}

async function readJsonObject(file: string): Promise<Record<string, unknown>> {
  const raw = JSON.parse(await readFile(file, 'utf8')) as unknown;
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
    throw new Error(`${path.basename(file)} is not a JSON object`);
  }
  return raw as Record<string, unknown>;
}

function stringField(tag: Record<string, unknown>, key: string): string | undefined {
  const value = tag[key];
  if (typeof value !== 'string') return undefined;
  const trimmed = value.trim();
  return trimmed.length === 0 ? undefined : trimmed;
}

function stringArrayField(tag: Record<string, unknown>, key: string): readonly string[] | undefined {
  const value = tag[key];
  if (!Array.isArray(value) || !value.every((entry) => typeof entry === 'string')) return undefined;
  return value as readonly string[];
}

function isWithin(child: string, parent: string): boolean {
  const relative = path.relative(parent, child);
  return relative === '' || (!relative.startsWith('..') && !path.isAbsolute(relative));
}

async function isFile(p: string): Promise<boolean> {
  try {
    return (await stat(p)).isFile();
  } catch {
    return false;
  }
}

async function isDir(p: string): Promise<boolean> {
  try {
    return (await stat(p)).isDirectory();
  } catch {
    return false;
  }
}
