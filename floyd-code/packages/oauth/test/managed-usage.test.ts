import { afterEach, describe, it, expect, vi } from 'vitest';

import {
  fetchManagedUsage,
  formatDuration,
  isManagedFloydCode,
  isManagedFloydCodeBaseUrl,
  floydCodeBaseUrl,
  floydCodeUsageUrl,
  managedUsageResultSchema,
  parseManagedUsagePayload,
} from '../src/managed-usage';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

describe('floydCodeBaseUrl', () => {
  it('strips trailing slashes from the FLOYD_CODE_BASE_URL override', () => {
    // The env value must be normalized at the source: provision persists it
    // verbatim while the model refresh rewrites it normalized, and the
    // deep-equal diff between the two shapes would fire a spurious
    // providers-changed event mid-login.
    vi.stubEnv('FLOYD_CODE_BASE_URL', 'https://gw.example.com/');
    expect(floydCodeBaseUrl()).toBe('https://gw.example.com');
    expect(floydCodeUsageUrl()).toBe('https://gw.example.com/usages');
  });

  it('resolves the base and the usage URL to empty without configuration', () => {
    vi.stubEnv('FLOYD_CODE_BASE_URL', undefined);
    expect(floydCodeBaseUrl()).toBe('');
    expect(floydCodeUsageUrl()).toBe('');
  });
});

describe('isManagedFloydCodeBaseUrl', () => {
  it('matches both official managed endpoints, with or without a trailing slash', () => {
    expect(isManagedFloydCodeBaseUrl('https://api.floyd.com/coding/v1')).toBe(true);
    expect(isManagedFloydCodeBaseUrl('https://api.floyd.com/coding/v1/')).toBe(true);
    expect(isManagedFloydCodeBaseUrl('https://api.floyd.ai/coding/v1')).toBe(true);
    expect(isManagedFloydCodeBaseUrl('https://api.floyd.ai/coding/v1/')).toBe(true);
  });

  it('matches against the FLOYD_CODE_BASE_URL override as the sole benchmark', () => {
    vi.stubEnv('FLOYD_CODE_BASE_URL', 'https://gw.example.com/coding/v1/');
    expect(isManagedFloydCodeBaseUrl('https://gw.example.com/coding/v1')).toBe(true);
    expect(isManagedFloydCodeBaseUrl('https://api.floyd.com/coding/v1')).toBe(false);
    expect(isManagedFloydCodeBaseUrl('https://api.floyd.ai/coding/v1')).toBe(false);
  });

  it('is case-insensitive on the origin but strict on the path', () => {
    expect(isManagedFloydCodeBaseUrl('https://API.FLOYD.COM/coding/v1')).toBe(true);
    expect(isManagedFloydCodeBaseUrl('https://api.floyd.com/CODING/v1')).toBe(false);
  });

  it('rejects other paths on the managed host and other hosts entirely', () => {
    expect(isManagedFloydCodeBaseUrl('https://api.floyd.com/coding/v2')).toBe(false);
    expect(isManagedFloydCodeBaseUrl('https://api.floyd.com/v1')).toBe(false);
    expect(isManagedFloydCodeBaseUrl('https://gateway.example.com/coding/v1')).toBe(false);
    expect(isManagedFloydCodeBaseUrl('https://api.legacy.cn/v1')).toBe(false);
  });

  it('rejects undefined and unparseable values', () => {
    expect(isManagedFloydCodeBaseUrl(undefined)).toBe(false);
    expect(isManagedFloydCodeBaseUrl('')).toBe(false);
    expect(isManagedFloydCodeBaseUrl('not a url')).toBe(false);
  });
});

describe('isManagedFloydCode', () => {
  it('matches only the floyd-code managed provider', () => {
    expect(isManagedFloydCode('managed:floyd-code')).toBe(true);
    expect(isManagedFloydCode('managed:legacy-ai')).toBe(false);
    expect(isManagedFloydCode('openai')).toBe(false);
    expect(isManagedFloydCode('')).toBe(false);
    expect(isManagedFloydCode(null)).toBe(false);
    expect(isManagedFloydCode()).toBe(false);
  });
});

describe('formatDuration', () => {
  it('formats days/hours/minutes', () => {
    expect(formatDuration(0)).toBe('0s');
    expect(formatDuration(45)).toBe('45s');
    expect(formatDuration(90)).toBe('1m');
    expect(formatDuration(3600)).toBe('1h');
    expect(formatDuration(3661)).toBe('1h 1m');
    expect(formatDuration(86_400 + 7200 + 600)).toBe('1d 2h 10m');
  });
});

describe('parseManagedUsagePayload', () => {
  it('degrades a non-record payload to zero values', () => {
    const zero = { usages: {}, extraUsage: null };
    expect(parseManagedUsagePayload(null)).toEqual(zero);
    expect(parseManagedUsagePayload('nope')).toEqual(zero);
    expect(parseManagedUsagePayload([])).toEqual(zero);
  });

  it('parses the full payload', () => {
    const parsed = parseManagedUsagePayload({
      goods_version: 2,
      usages: {
        limit_5h: { used_ratio: 0.3, reset_time: '2026-09-11T18:00:00Z' },
        limit_7d: { used_ratio: 0.2, reset_time: '2026-09-17T00:00:00Z' },
        limit_month_total: { used_ratio: 0.4, reset_time: '2026-10-01T00:00:00Z' },
        limit_month_code: { used_ratio: 0.25, reset_time: '2026-10-01T00:00:00Z' },
      },
    });
    expect(parsed).toEqual({
      usages: {
        limit5h: { usedRatio: 0.3, resetAt: '2026-09-11T18:00:00Z' },
        limit7d: { usedRatio: 0.2, resetAt: '2026-09-17T00:00:00Z' },
        monthTotal: { usedRatio: 0.4, resetAt: '2026-10-01T00:00:00Z' },
        monthCode: { usedRatio: 0.25, resetAt: '2026-10-01T00:00:00Z' },
      },
      extraUsage: null,
    });
  });

  it('drops entries that are not records or lack a numeric used_ratio', () => {
    const parsed = parseManagedUsagePayload({
      usages: {
        limit_5h: 'half',
        limit_7d: { reset_time: '2026-09-17T00:00:00Z' },
        limit_month_total: { used_ratio: Number.NaN },
        limit_month_code: { used_ratio: '0.25' },
      },
    });
    expect(parsed.usages).toEqual({
      limit5h: undefined,
      limit7d: undefined,
      monthTotal: undefined,
      monthCode: { usedRatio: 0.25, resetAt: undefined },
    });
  });

  it('keeps reset_time only when it is a non-empty string', () => {
    const parsed = parseManagedUsagePayload({
      usages: {
        limit_5h: { used_ratio: 0.3, reset_time: '' },
        limit_7d: { used_ratio: 0.2, reset_time: 42 },
      },
    });
    expect(parsed.usages.limit5h?.resetAt).toBeUndefined();
    expect(parsed.usages.limit7d?.resetAt).toBeUndefined();
  });

  it('parses the booster wallet from the usage payload', () => {
    const parsed = parseManagedUsagePayload({
      goods_version: 1,
      boosterWallet: {
        balance: {
          type: 'BOOSTER',
          amount: '20000000000',
          amountLeft: '10000000000',
        },
        monthlyChargeLimitEnabled: true,
        monthlyChargeLimit: { currency: 'CNY', priceInCents: '20000' },
        monthlyUsed: { currency: 'CNY', priceInCents: '5000' },
      },
    });
    expect(parsed.extraUsage).toEqual({
      balanceCents: 10000,
      totalCents: 20000,
      monthlyChargeLimitEnabled: true,
      monthlyChargeLimitCents: 20000,
      monthlyUsedCents: 5000,
      currency: 'CNY',
    });
  });

  it('drops the booster wallet when the balance is missing or not a booster', () => {
    expect(parseManagedUsagePayload({ boosterWallet: {} }).extraUsage).toBeNull();
    expect(
      parseManagedUsagePayload({ boosterWallet: { balance: { type: 'PLAN' } } }).extraUsage,
    ).toBeNull();
  });
});

describe('managedUsageResultSchema', () => {
  it('accepts the camelCase ok and error payloads', () => {
    const ok = {
      kind: 'ok' as const,
      quota: {
        usages: {
          limit5h: { usedRatio: 0.3, resetAt: '2026-09-11T18:00:00Z' },
          monthTotal: { usedRatio: 0.4 },
        },
        extraUsage: null,
      },
    };
    expect(managedUsageResultSchema.parse(ok)).toEqual(ok);
    expect(
      managedUsageResultSchema.parse({ kind: 'error', message: 'nope', status: 401 }),
    ).toEqual({ kind: 'error', message: 'nope', status: 401 });
  });
});

describe('fetchManagedUsage', () => {
  it('sends only Authorization and Accept headers', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            goods_version: 2,
            usages: { limit_5h: { used_ratio: 0.3, reset_time: '2026-09-11T18:00:00Z' } },
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchManagedUsage('https://api.example/usages', 'access-token')).resolves.toEqual({
      kind: 'ok',
      quota: {
        usages: {
          limit5h: { usedRatio: 0.3, resetAt: '2026-09-11T18:00:00Z' },
          limit7d: undefined,
          monthTotal: undefined,
          monthCode: undefined,
        },
        extraUsage: null,
      },
    });

    const calls = fetchMock.mock.calls as unknown as [string, RequestInit?][];
    const init = calls[0]?.[1] ?? {};
    const headers = new Headers((init.headers ?? {}) as Record<string, string>);
    expect(headers.get('authorization')).toBe('Bearer access-token');
    expect(headers.get('accept')).toBe('application/json');
    expect(headers.get('user-agent')).toBeNull();
    expect(headers.get('x-msh-platform')).toBeNull();
  });

  it('surfaces JSON API error messages with status', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(JSON.stringify({ message: 'token expired' }), {
            status: 401,
            headers: { 'Content-Type': 'application/json' },
          }),
      ),
    );

    const result = await fetchManagedUsage('https://api.example/usages', 'access-token');

    expect(result.kind).toBe('error');
    if (result.kind !== 'error') return;
    expect(result.status).toBe(401);
    expect(result.message).toBe('token expired');
  });

  it('falls back to the local usage hint on an empty 404 body', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('', { status: 404 })));

    const result = await fetchManagedUsage('https://api.example/usages', 'access-token');

    expect(result).toEqual({
      kind: 'error',
      status: 404,
      message: 'Usage endpoint not available. Try Floyd For Coding.',
    });
  });

  it('maps an aborted request to a timeout message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        const error = new Error('The operation was aborted');
        error.name = 'AbortError';
        throw error;
      }),
    );

    const result = await fetchManagedUsage('https://api.example/usages', 'access-token');

    expect(result).toEqual({ kind: 'error', message: 'Failed to fetch usage: request timed out.' });
  });

  it('wraps network failures with the usage prefix', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('socket hang up');
      }),
    );

    const result = await fetchManagedUsage('https://api.example/usages', 'access-token');

    expect(result).toEqual({ kind: 'error', message: 'Failed to fetch usage: socket hang up' });
  });

  it('does not request an unconfigured base URL', async () => {
    vi.stubEnv('FLOYD_CODE_BASE_URL', undefined);
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchManagedUsage(floydCodeUsageUrl(), 'access-token');

    expect(result.kind).toBe('error');
    if (result.kind !== 'error') return;
    expect(result.message).toBe(
      'Failed to fetch usage: no managed base URL is configured. Set FLOYD_CODE_BASE_URL.',
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
