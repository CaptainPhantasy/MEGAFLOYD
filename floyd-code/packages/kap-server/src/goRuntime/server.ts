import { join, resolve } from 'node:path';
import { readFile } from 'node:fs/promises';
import { parse as parseToml } from 'smol-toml';
import Fastify, { type FastifyInstance } from 'fastify';
import type { Scope } from '@legacy-ai/agent-core-v2';
import type { RunningServer, ServerStartOptions } from '../start';
import { createServerLogger } from '../services/pinoLoggerService';
import { createHostCheck, isHostCheckDisabled, parseAllowedHosts } from '../middleware/hostnames';
import { createOriginHook, isOriginAllowed, parseCorsOrigins } from '../middleware/origin';
import { registerWebAssetRoutes } from '../routes/webAssets';
import { registerGuiStoreRoutes } from '../routes/guiStore';
import { GuiStoreService } from '../services/guiStore/guiStoreService';
import { ConnectionRegistry } from '../transport/ws/connectionRegistry';
import { GoRuntime } from './runtime';
import { registerGoRoutes } from './routes';
import { createGoWebSocket } from './websocket';
import { LocalViews } from './localViews';
import { GoWorkspaces } from './workspaces';
import { GoAttachments, registerGoAttachments } from './attachments';
import { GoProviders } from './providers';
import { GoPlugins } from './plugins';
import { GoMcp } from './mcp';
import { GoRequestError } from './client';
import { GoOAuth } from './oauth';

export async function startGoServer(opts: ServerStartOptions): Promise<RunningServer> {
  const env = opts.env ?? process.env;
  process.title = 'megafloyd-web';
  const goHost = env['MEGAFLOYD_GO_HOST'];
  const cwd = env['MEGAFLOYD_GO_CWD'];
  const dataDir = env['MEGAFLOYD_GO_DATA_DIR'];
  if (!goHost || !cwd || !dataDir) throw new Error('Shared Go web mode requires MEGAFLOYD_GO_HOST, MEGAFLOYD_GO_CWD, and MEGAFLOYD_GO_DATA_DIR');
  if (!opts.homeDir && !env['FLOYD_CODE_HOME']) throw new Error('Shared Go web mode requires a canonical web home');
  const homeDir = resolve(opts.homeDir || env['FLOYD_CODE_HOME']!);
  if (env['MEGAFLOYD_HOME'] && homeDir !== resolve(env['MEGAFLOYD_HOME'], 'web')) throw new Error('Shared Go web home must be MEGAFLOYD_HOME/web');
  try { parseToml(await readFile(join(homeDir, 'gui.toml'), 'utf8')); } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
  const host = opts.host ?? '127.0.0.1';
  const logger = opts.logger ?? createServerLogger({ level: opts.logLevel ?? 'warn' });
  const app = Fastify({ loggerInstance: logger, disableRequestLogging: true }) as unknown as FastifyInstance;
  app.setValidatorCompiler(() => () => true);
  app.setSerializerCompiler(() => (data) => JSON.stringify(data));
  const runtime = new GoRuntime(goHost, resolve(cwd), resolve(dataDir));
  const views = new LocalViews(runtime, homeDir);
  const workspaces = new GoWorkspaces(runtime, views, homeDir);
  const attachments = new GoAttachments(join(dataDir, 'attachments'));
  const companyBase = (env['FLOYD_CODE_BASE_URL'] || '').replace(/\/+$/, '');
  const oauth = new GoOAuth(env);
  const providers = new GoProviders(env['MEGAFLOYD_CATALOG_URL'] || (companyBase ? `${companyBase}/catalog/api.json` : ''), (selected, id) => oauth.refreshProvider(selected, id));
  const plugins = new GoPlugins(resolve(env['MEGAFLOYD_HOME'] || join(homeDir, 'canonical')), workspaces, env['MEGAFLOYD_PLUGIN_MARKETPLACE_URL'] || '');
  const mcp = new GoMcp(plugins);
  runtime.attachments = attachments;
  workspaces.onAttach = async (attached) => { attached.attachments = attachments; await plugins.sync(attached); };
  app.setErrorHandler((error, req, reply) => {
    const status = error instanceof GoRequestError ? error.status : Number((error as { statusCode?: number }).statusCode) || 500;
    void reply.code(status).send({ code: status * 100, msg: error instanceof Error ? error.message : String(error), data: null, request_id: req.id });
  });
  const registry = new ConnectionRegistry();
  const hostCheck = createHostCheck({ boundHost: host, extra: [...parseAllowedHosts(env), ...(opts.allowedHosts ?? [])], disable: opts.disableHostCheck ?? isHostCheckDisabled() });
  const allowedOrigins = opts.corsOrigins ?? parseCorsOrigins();
  app.addHook('onRequest', hostCheck.onRequest);
  app.addHook('onRequest', createOriginHook({ allowedOrigins }));
  let closing: Promise<void> | undefined;
  const wss = createGoWebSocket(runtime, registry, views, workspaces);
  const close = (): Promise<void> => {
    closing ??= (async () => { for (const socket of wss.clients) socket.terminate(); wss.close(); mcp.close(); oauth.close(); await workspaces.close(); await app.close(); })();
    return closing;
  };
  try {
    await runtime.start();
    await workspaces.load();
    await plugins.load();
    await plugins.sync(runtime);
    registerGoAttachments(app, attachments, workspaces);
    await app.register(async (api) => registerGuiStoreRoutes(api, new GuiStoreService(homeDir, logger)), { prefix: '/api/v1' });
    registerGoRoutes(app, runtime, views, { version: opts.serverVersion ?? 'megafloyd-go', startedAt: new Date().toISOString(), title: opts.webTitle, workspaces, providers, plugins, mcp, oauth, shutdown: () => { setImmediate(() => { void close(); }); } });
    app.server.on('upgrade', (req, socket, head) => {
      const path = new URL(req.url ?? '/', 'http://web').pathname;
      if (path !== '/api/v1/ws' || !hostCheck.isAllowed(req.headers.host) || !isOriginAllowed(req.headers.origin, req.headers.host, allowedOrigins)) { socket.write('HTTP/1.1 403 Forbidden\r\nConnection: close\r\n\r\n'); socket.destroy(); return; }
      wss.handleUpgrade(req, socket, head, (connection) => wss.emit('connection', connection, req));
    });
    if (opts.webAssetsDir) await registerWebAssetRoutes(app, opts.webAssetsDir);
    await app.listen({ host, port: opts.port ?? 58627 });
    const address = app.server.address();
    if (!address || typeof address === 'string') throw new Error('Canonical web server did not bind a TCP port');
    await runtime.client.json('POST', '/v1/handoff/web', { url: `http://127.0.0.1:${address.port}` });
  } catch (error) { await close(); throw error; }
  const address = app.server.address();
  const port = typeof address === 'object' && address !== null ? address.port : opts.port ?? 58627;
  return { app, get core(): Scope { throw new Error('This web server uses the shared Go runtime; no TypeScript agent scope exists'); }, connectionRegistry: registry, authTokenService: { _serviceBrand: undefined, getToken: () => '', isValid: async () => true }, host, port, close };
}
