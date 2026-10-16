#!/usr/bin/env node
// Stdio MCP server for floyd-datasource.
//
// Speaks newline-delimited JSON-RPC 2.0 on stdin/stdout per the MCP "stdio"
// transport. Implements the minimal surface the Floyd Code host calls:
//   - initialize
//   - notifications/initialized
//   - tools/list
//   - tools/call
//   - ping
//
// Business logic is kept self-contained so the plugin can run from a zipped
// marketplace install without workspace package dependencies.

import { createHash, randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { arch, homedir, hostname, release, type } from 'node:os';
import path from 'node:path';
import readline from 'node:readline';

const VERSION = '3.4.0';
const DEFAULT_CREDENTIAL_NAME = 'floyd-code';
const REQUEST_TIMEOUT_MS = 30_000;
const PROTOCOL_VERSION = '2025-06-18';

const TOOLS = [
  {
    name: 'call_data_source_tool',
    description:
      "Dispatch one call to the data source selected for the user's request. Always call get_data_source_desc(name) first, then use an api_name and params from that description. For a simple lookup, use one specialized source and stop once a result covers the user's question; do not query fallback or comparison sources unless the user explicitly asks for a cross-source comparison. When the user names a data source, use that source.",
    inputSchema: {
      type: 'object',
      properties: {
        data_source_name: {
          type: 'string',
          description:
            'The data source selected via get_data_source_desc. When the user names a source, pass that source.',
        },
        api_name: {
          type: 'string',
          description: 'API name from the data source description.',
        },
        params: {
          type: 'object',
          description: 'API parameters that match the data source description.',
        },
      },
      required: ['data_source_name', 'api_name', 'params'],
    },
  },
  {
    name: 'get_data_source_desc',
    description:
      'Get the current API documentation for one Floyd data source before calling a specific API. For a simple lookup, choose exactly one specialized source; do not inspect fallback or comparison sources unless the user explicitly asks for a cross-source comparison.',
    inputSchema: {
      type: 'object',
      properties: {
        name: {
          type: 'string',
          enum: [
            'stock_finance_data',
            'yahoo_finance',
            'world_bank_open_data',
            'tianyancha',
            'arxiv',
            'scholar',
            'yuandian_law',
            'wind',
            'imf',
            'gildata',
            'sec_edgar',
            'sp_data',
            'china_nda',
            'china_nbs',
            'china_standards',
            'who',
            'fao',
            'unsd',
            'ecb',
            'eurostat',
            'unicef',
            'oecd',
            'fred',
            'xhcj',
            'caixin',
          ],
          description:
            'Data source name. Capabilities: stock_finance_data / yahoo_finance = general quotes and financials ' +
            '(yahoo_finance FX history is limited to about 2 years); world_bank_open_data = historical macro; ' +
            'imf = FX rates, CPI, GDP forecasts, balance of payments; tianyancha = CN company registry; ' +
            'arxiv / scholar = papers; yuandian_law = CN laws and cases; ' +
            'wind = A-share intraday minute series, funds, bonds (map PE/PB/ROE-style field names via wind_search_fields first); ' +
            'gildata = natural-language stock/fund screening; ' +
            'sec_edgar = US filings (10-K/10-Q, S-1, Form 4, 13F, 8-K); ' +
            'sp_data = S&P fundamentals (consensus estimates, valuation ratios, transcripts); ' +
            'china_nda = CN government open data catalogs (National Data Administration registry + provincial platforms); ' +
            'china_nbs = CN NBS macro indicators and time series (national / provincial / major-city scopes); ' +
            'china_standards = CN standards (GB national, HB industry, DB local, TT association); ' +
            'who / fao / unsd / ecb / eurostat / unicef / oecd / fred = international organization open data ' +
            '(global health, food & agriculture, UN statistics, ECB & EU statistics, child indicators, OECD datasets, US & global macro series); ' +
            'xhcj = Xinhua Finance (CNFIC) news flashes, announcements, and policies; ' +
            'caixin = Caixin database (600+ data APIs, discover via caixin_api_search first).',
        },
      },
      required: ['name'],
    },
  },
];

const HANDLERS = {
  call_data_source_tool: {
    method: 'call_data_source_tool',
    buildParams(args) {
      return {
        data_source_name: requiredString(args, 'data_source_name'),
        api_name: requiredString(args, 'api_name'),
        params: requiredObject(args, 'params'),
      };
    },
  },
  get_data_source_desc: {
    method: 'get_data_source_desc',
    buildParams(args) {
      return { name: requiredString(args, 'name') };
    },
  },
};

async function handleRequest(message) {
  const { method, id, params } = message;
  switch (method) {
    case 'initialize':
      return {
        protocolVersion: PROTOCOL_VERSION,
        capabilities: { tools: {} },
        serverInfo: { name: 'floyd-datasource', version: VERSION },
      };
    case 'ping':
      return {};
    case 'tools/list':
      return { tools: TOOLS };
    case 'tools/call':
      return runTool(params);
    default:
      throw jsonRpcError(-32601, `Method not found: ${method}`, { id });
  }
}

async function runTool(params) {
  const name = params?.name;
  const args = params?.arguments ?? {};
  const handler = HANDLERS[name];
  if (handler === undefined) {
    return {
      content: [{ type: 'text', text: `Unknown tool: ${String(name)}` }],
      isError: true,
    };
  }
  const trace = {};
  try {
    const built = handler.buildParams(args);
    const response = await callFloydTool(handler.method, built, trace);
    const fileWarnings = await writeResponseFiles(response, expectedResponseFilePath(built));
    const text = extractText(response);
    const formatted = (handler.format?.(text, built) ?? text).trim();
    return { content: [{ type: 'text', text: appendTrace(appendWarnings(formatted, fileWarnings), trace) }] };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return {
      content: [{ type: 'text', text: appendTrace(message, trace) }],
      isError: true,
    };
  }
}

async function writeResponseFiles(response, expectedOutputPath) {
  if (!isRecord(response) || !Array.isArray(response.files)) return [];
  const warnings = [];

  for (const file of response.files) {
    if (!isRecord(file)) continue;
    const name = typeof file.name === 'string' ? file.name.trim() : '';
    if (name.length === 0 || file.content === undefined || file.content === null) continue;

    const writePath = allowedResponseFilePath(name, expectedOutputPath);
    if (writePath === undefined) {
      warnings.push(`Warning: skipped returned file ${name} because it is outside the requested output path.`);
      continue;
    }

    try {
      await mkdir(path.dirname(writePath), { recursive: true });
      if (file.encoding === 'base64') {
        await writeFile(writePath, Buffer.from(String(file.content), 'base64'));
      } else {
        await writeFile(writePath, String(file.content), 'utf8');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      warnings.push(`Warning: failed to write file ${writePath}: ${message}`);
    }
  }

  return warnings;
}

function expectedResponseFilePath(params) {
  return outputPathField(params) ?? (isRecord(params) ? outputPathField(params.params) : undefined);
}

function outputPathField(value) {
  if (!isRecord(value)) return undefined;
  for (const field of ['file_path', 'filepath']) {
    const pathValue = value[field];
    if (typeof pathValue !== 'string') continue;
    const trimmed = pathValue.trim();
    if (trimmed.length > 0) return trimmed;
  }
  return undefined;
}

function allowedResponseFilePath(name, expectedOutputPath) {
  if (expectedOutputPath === undefined) return undefined;

  const actual = path.resolve(name);
  const expected = path.resolve(expectedOutputPath);
  if (actual === expected) return actual;

  const actualParts = path.parse(actual);
  const expectedParts = path.parse(expected);
  if (actualParts.dir !== expectedParts.dir) return undefined;
  if (actualParts.ext !== expectedParts.ext) return undefined;
  if (!actualParts.name.startsWith(`${expectedParts.name}_`)) return undefined;

  return actual;
}

function appendWarnings(text, warnings) {
  if (warnings.length === 0) return text;
  return `${text}\n\n${warnings.join('\n')}`;
}

// Pick the backend request id from the response headers, if the gateway sends one.
function extractRequestId(headers) {
  for (const key of ['x-request-id', 'x-trace-id', 'x-msh-request-id', 'x-msh-trace-id', 'request-id']) {
    const value = headers.get(key);
    if (typeof value === 'string' && value.trim().length > 0) return value.trim();
  }
  return undefined;
}

// Append a trace line so failures can be correlated with backend logs. The
// tool-call-id is the `X-Msh-Tool-Call-Id` header we send on every request.
function appendTrace(text, trace) {
  if (trace === undefined || trace.toolCallId === undefined) return text;
  const parts = [];
  if (trace.requestId !== undefined) parts.push(`request-id: ${trace.requestId}`);
  parts.push(`tool-call-id: ${trace.toolCallId}`);
  return `${text}\n\n[floyd-datasource] ${parts.join(' · ')}`;
}

function resolveFloydHome() {
  const explicit = process.env.FLOYD_CODE_HOME?.trim();
  return explicit && explicit.length > 0 ? explicit : path.join(homedir(), '.floyd-code');
}

function datasourceApiUrl() {
  const explicit = process.env.FLOYD_DATASOURCE_API_URL?.trim();
  if (explicit !== undefined && explicit.length > 0) return explicit;
  const baseUrl = floydCodeBaseUrl();
  if (baseUrl === undefined) {
    throw new Error(
      'Floyd datasource is not configured: set FLOYD_CODE_BASE_URL to the Floyd Code API base URL ' +
        '(tool calls go to <base>/tools), or set FLOYD_DATASOURCE_API_URL to the full tool endpoint.',
    );
  }
  return `${baseUrl}/tools`;
}

function floydCodeBaseUrl() {
  return configuredEndpoint(process.env.FLOYD_CODE_BASE_URL);
}

function floydCodeOAuthHost() {
  return configuredEndpoint(process.env.FLOYD_CODE_OAUTH_HOST ?? process.env.FLOYD_OAUTH_HOST);
}

function configuredEndpoint(value) {
  if (value === undefined) return undefined;
  const normalized = value.trim().replace(/\/+$/, '');
  return normalized.length > 0 ? normalized : undefined;
}

function resolveFloydCodeCredentialName() {
  const oauthHost = floydCodeOAuthHost();
  const baseUrl = floydCodeBaseUrl();
  if (oauthHost === undefined && baseUrl === undefined) return DEFAULT_CREDENTIAL_NAME;
  if (oauthHost === undefined || baseUrl === undefined) {
    throw new Error(
      `Floyd datasource credentials are scoped to an (API base URL, OAuth host) pair: set FLOYD_CODE_OAUTH_HOST ` +
        `(or FLOYD_OAUTH_HOST) alongside FLOYD_CODE_BASE_URL so the credential file name resolves, or leave both ` +
        `unset to use the default ${DEFAULT_CREDENTIAL_NAME} credentials.`,
    );
  }

  // Keep this in sync with packages/oauth/src/managed-floyd-code.ts.
  const digest = createHash('sha256')
    .update(JSON.stringify({ oauthHost, baseUrl }))
    .digest('hex')
    .slice(0, 16);
  return `floyd-code-env-${digest}`;
}

async function loadAccessToken() {
  const floydHome = resolveFloydHome();
  const credentialsFile = path.join(
    floydHome,
    'credentials',
    `${resolveFloydCodeCredentialName()}.json`,
  );
  let parsed;
  try {
    parsed = JSON.parse(await readFile(credentialsFile, 'utf8'));
  } catch (error) {
    if (isNotFound(error)) {
      throw new Error(
        `Floyd Code credentials file not found: ${credentialsFile}\nRun /login in Floyd Code first.`,
      );
    }
    if (error instanceof SyntaxError) {
      throw new Error(`Failed to parse Floyd Code credentials file: ${error.message}`);
    }
    throw error;
  }

  if (!isRecord(parsed)) {
    throw new Error(`Invalid Floyd Code credentials file: ${credentialsFile}`);
  }
  const token = typeof parsed.access_token === 'string' ? parsed.access_token : '';
  if (token.length === 0) {
    throw new Error('Floyd Code credentials do not contain access_token. Run /login again.');
  }
  return { floydHome, token };
}

async function callFloydTool(method, params, trace = {}) {
  const apiUrl = datasourceApiUrl();
  const { floydHome, token: initialToken } = await loadAccessToken();
  let token = initialToken;
  const toolCallId = randomUUID();
  trace.toolCallId = toolCallId;
  const controller = new AbortController();
  const timeout = setTimeout(() => {
    controller.abort();
  }, REQUEST_TIMEOUT_MS);
  try {
    const request = async (accessToken) => {
      const response = await fetch(apiUrl, {
        method: 'POST',
        headers: await buildHeaders(floydHome, accessToken, toolCallId),
        body: JSON.stringify({ method, params }),
        signal: controller.signal,
      });
      return { response, text: await response.text() };
    };

    let { response, text } = await request(token);
    if (response.status === 401) {
      const refreshed = await loadAccessToken();
      if (refreshed.token !== token) {
        token = refreshed.token;
        ({ response, text } = await request(token));
      }
    }
    trace.requestId = extractRequestId(response.headers);
    if (!response.ok) {
      if (response.status === 401) {
        throw new Error('Floyd Code access_token was rejected. Run /login again and retry.');
      }
      throw new Error(`HTTP ${response.status} error: ${text}`);
    }
    try {
      return JSON.parse(text);
    } catch {
      return text;
    }
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new Error(`Request timed out after ${REQUEST_TIMEOUT_MS / 1000} seconds.`);
    }
    throw error;
  } finally {
    clearTimeout(timeout);
  }
}

async function buildHeaders(floydHome, token, toolCallId) {
  return {
    Authorization: `Bearer ${token}`,
    'Content-Type': 'application/json',
    'X-Msh-Tool-Call-Id': toolCallId,
    'X-Msh-Platform': asciiHeader(process.env.FLOYD_MSH_PLATFORM ?? 'floyd-code-cli'),
    'X-Msh-Version': asciiHeader(process.env.FLOYD_MSH_VERSION ?? VERSION),
    'X-Msh-Device-Name': asciiHeader(process.env.FLOYD_MSH_DEVICE_NAME ?? hostname()),
    'X-Msh-Device-Model': asciiHeader(process.env.FLOYD_MSH_DEVICE_MODEL ?? deviceModel()),
    'X-Msh-Os-Version': asciiHeader(process.env.FLOYD_MSH_OS_VERSION ?? release()),
    'X-Msh-Device-Id': asciiHeader(process.env.FLOYD_MSH_DEVICE_ID ?? (await createDeviceId(floydHome))),
    'User-Agent': `floyd-datasource/${VERSION}`,
  };
}

async function createDeviceId(floydHome) {
  const deviceIdPath = path.join(floydHome, 'device_id');
  try {
    const existing = (await readFile(deviceIdPath, 'utf8')).trim();
    if (existing.length > 0) return existing;
  } catch {
    // Fall through to create a best-effort local device id.
  }

  const id = randomUUID();
  try {
    await mkdir(floydHome, { recursive: true, mode: 0o700 });
    await writeFile(deviceIdPath, `${id}\n`, { encoding: 'utf8', mode: 0o600 });
  } catch {
    // Headers can still use the in-memory id if the file cannot be written.
  }
  return id;
}

function deviceModel() {
  const os = type();
  const osVersion = release();
  const osArch = arch();
  if (os === 'Darwin') return `macOS ${osVersion} ${osArch}`;
  if (os === 'Windows_NT') return `Windows ${osVersion} ${osArch}`;
  return `${os} ${osVersion} ${osArch}`.trim();
}

function extractText(response) {
  if (typeof response === 'string') return response;
  if (!isRecord(response)) return String(response);

  if (response.is_success === false) {
    const message = extractChannelText(response.error) ?? JSON.stringify(response);
    throw new Error(`Tool API returned an error: ${message}`);
  }

  const text = extractChannelText(response.result);
  if (text !== undefined) return text;
  return `Tool API succeeded but did not return user text. Raw response: ${JSON.stringify(response)}`;
}

function extractChannelText(value) {
  if (!isRecord(value)) return undefined;
  for (const channel of ['assistant', 'user']) {
    const items = value[channel];
    if (!Array.isArray(items)) continue;
    const text = items
      .filter((item) => isRecord(item) && item.type === 'text' && typeof item.text === 'string')
      .map((item) => item.text)
      .filter(Boolean)
      .join('\n\n')
      .trim();
    if (text.length > 0) return text;
  }
  return undefined;
}

function requiredString(args, field) {
  const value = optionalString(args, field);
  if (value === undefined) throw new Error(`Missing required argument: ${field}.`);
  return value;
}

function optionalString(args, field) {
  if (!isRecord(args)) return undefined;
  const value = args[field];
  if (value === undefined || value === null) return undefined;
  if (typeof value !== 'string') throw new Error(`${field} must be a string.`);
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

function requiredObject(args, field) {
  if (!isRecord(args)) throw new Error(`Missing required argument: ${field}.`);
  const value = args[field];
  if (!isRecord(value)) throw new Error(`${field} must be an object.`);
  return value;
}

function isRecord(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isNotFound(err) {
  return isRecord(err) && err.code === 'ENOENT';
}

function asciiHeader(value, fallback = 'unknown') {
  const cleaned = String(value).replaceAll(/[^ -~]/g, '').trim();
  return cleaned.length > 0 ? cleaned : fallback;
}

function jsonRpcError(code, message, data) {
  const err = new Error(message);
  err.jsonRpc = { code, message, data };
  return err;
}

function send(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

function sendResult(id, result) {
  send({ jsonrpc: '2.0', id, result });
}

function sendError(id, error) {
  send({ jsonrpc: '2.0', id, error });
}

async function dispatch(message) {
  if (message?.jsonrpc !== '2.0') return;
  // Notifications carry no id and never expect a response.
  if (message.id === undefined || message.id === null) {
    if (message.method === 'notifications/initialized' || message.method === 'notifications/cancelled') {
      return;
    }
    return;
  }
  const id = message.id;
  try {
    const result = await handleRequest(message);
    sendResult(id, result ?? {});
  } catch (error) {
    if (error && typeof error === 'object' && error.jsonRpc !== undefined) {
      sendError(id, error.jsonRpc);
      return;
    }
    sendError(id, {
      code: -32603,
      message: error instanceof Error ? error.message : String(error),
    });
  }
}

function start() {
  const rl = readline.createInterface({ input: process.stdin });
  rl.on('line', (line) => {
    const trimmed = line.trim();
    if (trimmed.length === 0) return;
    let message;
    try {
      message = JSON.parse(trimmed);
    } catch (error) {
      sendError(null, {
        code: -32700,
        message: `Parse error: ${error instanceof Error ? error.message : String(error)}`,
      });
      return;
    }
    void dispatch(message);
  });
  rl.on('close', () => {
    process.exit(0);
  });
}

start();
