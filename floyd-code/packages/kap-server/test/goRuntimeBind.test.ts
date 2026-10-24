import { expect, test } from 'vitest';
import { startGoServer } from '../src/goRuntime/server';

test.each(['0.0.0.0', '::', '192.168.1.10', '100.78.109.105', 'example.com', ''])('shared account runtime rejects remote listener %s before opening state', async (host) => {
  await expect(startGoServer({
    host, port: 0, env: {},
    hostIdentity: { name: 'MegaFloyd', version: 'test' } as never,
    disableAuth: true, disableHostCheck: true, insecureNoTls: true,
    allowedHosts: ['*'], corsOrigins: ['*'],
  })).rejects.toThrow('Shared Go web mode requires a loopback listener');
});

test.each(['127.0.0.1', '127.0.0.2', '::1', 'localhost', undefined])('local listener %s proceeds to canonical runtime validation', async (host) => {
  await expect(startGoServer({
    host, port: 0, env: {},
    hostIdentity: { name: 'MegaFloyd', version: 'test' } as never,
  })).rejects.toThrow('Shared Go web mode requires MEGAFLOYD_GO_HOST');
});
