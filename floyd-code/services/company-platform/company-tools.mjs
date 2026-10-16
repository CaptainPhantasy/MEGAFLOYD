import { createHash } from 'node:crypto';
import { copyFile, mkdir, readFile, rename, writeFile } from 'node:fs/promises';
import { homedir } from 'node:os';
import { dirname, isAbsolute, join, resolve } from 'node:path';
import { createInterface } from 'node:readline';

const base = (process.env.FLOYD_CODE_BASE_URL || 'https://inference.tail58d565.ts.net:8446').replace(/\/+$/, '');
const oauthHost = (process.env.FLOYD_CODE_OAUTH_HOST || base).replace(/\/+$/, '');
const home = process.env.FLOYD_CODE_HOME || join(homedir(), '.floyd-code');
const digest = createHash('sha256').update(JSON.stringify({ oauthHost, baseUrl: base })).digest('hex').slice(0, 16);
const credential = join(home, 'credentials', `floyd-code-env-${digest}.json`);

async function request(path, body) {
  let saved;
  try { saved = JSON.parse(await readFile(credential, 'utf8')); }
  catch { throw new Error('Sign in with f7 login to use the company tools.'); }
  if (!saved.access_token) throw new Error('Sign in with f7 login to use the company tools.');
  const response = await fetch(base + path, {
    method: body === undefined ? 'GET' : 'POST',
    headers: { Authorization: `Bearer ${saved.access_token}`, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(55000),
  });
  const result = await response.json();
  if (!response.ok) throw new Error(response.status === 401 ? 'Sign in with f7 login, then retry.' : result.error_description || 'The company tool could not finish this request.');
  return result;
}

function destination(value) {
  if (typeof value !== 'string' || !value.trim()) throw new Error('Error: No destination folder specified. Supply an output path within your user space.');
  if (!isAbsolute(value)) throw new Error('Use the full output filename inside your own folder.');
  return resolve(value);
}

async function save(path, bytes) {
  await mkdir(dirname(path), { recursive: true, mode: 0o700 });
  await writeFile(path, bytes, { flag: 'wx', mode: 0o600 });
}

if (process.argv[2] === '--install-mcp') {
  const file = process.argv[3];
  if (!file || !isAbsolute(file)) throw new Error('Choose the full settings filename.');
  let previous = {}, exists = false;
  try { previous = JSON.parse(await readFile(file, 'utf8')); exists = true; }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
  if (previous === null || typeof previous !== 'object' || Array.isArray(previous)) throw new Error('The saved tool settings could not be read. They were kept unchanged.');
  if (previous.mcpServers !== undefined && (previous.mcpServers === null || typeof previous.mcpServers !== 'object' || Array.isArray(previous.mcpServers))) throw new Error('The saved tool settings could not be read. They were kept unchanged.');
  const entry = resolve(process.argv[1]);
  const root = dirname(entry);
  const command = process.env.FLOYD_COMPANY_COMMAND || process.execPath;
  const next = { ...previous, mcpServers: { ...previous.mcpServers, 'floyd-company': {
    command, args: ['__plugin_run_node', entry], env: {
      FLOYD_PLUGIN_ROOT: root, FLOYD_CODE_BASE_URL: base, FLOYD_CODE_OAUTH_HOST: oauthHost,
      FLOYD_CODE_CDN_BASE: 'https://inference.tail58d565.ts.net:8444/code', FLOYD_DISABLE_TELEMETRY: '1',
      FLOYD_CODE_HOME: home,
    },
  } } };
  await mkdir(dirname(file), { recursive: true, mode: 0o700 });
  if (exists) await copyFile(file, file + '.recovery-' + Date.now());
  const temporary = file + '.company-' + process.pid;
  await writeFile(temporary, JSON.stringify(next, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
  await rename(temporary, file);
  process.stdout.write('Company tools added. Your other tools were kept.\n');
} else {
  const text = value => ({ content: [{ type: 'text', text: typeof value === 'string' ? value : JSON.stringify(value) }] });
  const lines = createInterface({ input: process.stdin, crlfDelay: Infinity });
  for await (const line of lines) {
    let message;
    try {
      message = JSON.parse(line);
      if (message.id === undefined) continue;
      let result;
      if (message.method === 'initialize') result = { protocolVersion: message.params?.protocolVersion || '2024-11-05', capabilities: { tools: {} }, serverInfo: { name: 'floyd-company', version: '2.0.2' } };
      else if (message.method === 'ping') result = {};
      else if (message.method === 'tools/list') {
        const listed = (await request('/company/tools')).tools;
        const tools = listed.map(tool => ({ name: tool.id, description: tool.name, inputSchema: tool.schema?.inputSchema || { type: 'object', properties: {} } }));
        const speech = tools.find(tool => tool.name === 'speech-transcribe');
        if (speech) speech.inputSchema = { type: 'object', properties: { file: { type: 'string', description: 'Full recording filename on your own computer.' }, language: { type: 'string', default: 'en-US' }, destination: { type: 'string', description: 'Optional text output file on your own computer.' } }, required: ['file'] };
        if (tools.some(tool => tool.name === 'music-create')) tools.push({ name: 'music-check', description: 'Check a music request and save it in your own folder.', inputSchema: { type: 'object', properties: { job: { type: 'string' }, destination: { type: 'string' } }, required: ['job', 'destination'] } });
        result = { tools };
      } else if (message.method === 'tools/call') {
        const name = message.params.name, args = { ...message.params.arguments };
        try {
          if (name === 'music-check') {
            const path = destination(args.destination);
            if (typeof args.job !== 'string' || !/^[a-f0-9]{32}$/.test(args.job)) throw new Error('Choose a valid music request.');
            const found = await request('/company/media/jobs/' + args.job);
            if (found.status === 'complete') { await save(path, Buffer.from(found.audio_base64, 'base64')); result = text({ status: 'saved', file: path, bytes: found.bytes }); }
            else result = text({ status: found.status, message: found.message, job: args.job });
          } else {
            let output;
            if (name === 'speech-transcribe') {
              const file = destination(args.file);
              const audio = await readFile(file);
              if (audio.length > 22 * 1024 * 1024) throw new Error('Choose a recording smaller than 22 MB.');
              output = args.destination === undefined ? undefined : destination(args.destination);
              args.filename = file.split(/[\\/]/).pop(); args.audio_base64 = audio.toString('base64'); delete args.file; delete args.destination;
            }
            if (name === 'music-create') args.destination = destination(args.destination);
            const called = await request('/company/tools/call', { id: name, arguments: args });
            if (output) { await save(output, called.text + '\n'); called.file = output; }
            result = called.content ? called : text(called);
          }
        } catch (error) { result = { ...text(error.message), isError: true }; }
      } else throw new Error('That tool request is not supported.');
      process.stdout.write(JSON.stringify({ jsonrpc: '2.0', id: message.id, result }) + '\n');
    } catch (error) {
      process.stdout.write(JSON.stringify({ jsonrpc: '2.0', id: message?.id ?? null, error: { code: -32603, message: error.message } }) + '\n');
    }
  }
}
