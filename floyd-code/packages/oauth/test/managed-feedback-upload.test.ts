import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  fetchCompleteFeedbackUpload,
  fetchCreateFeedbackUploadUrl,
  floydCodeFeedbackUploadCompleteUrl,
  floydCodeFeedbackUploadUrl,
  type CreateFeedbackUploadUrlBody,
} from '../src/managed-feedback-upload';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

const SAMPLE_BODY: CreateFeedbackUploadUrlBody = {
  file_hash: 'e4d649659ca70729a510ef58f4cd062890020a1038eead5f411451fce62df415',
  file_name: 'repo.zip',
  file_size: 123,
  feedback_id: 3,
};

const CONFIGURED_BASE_URL = 'https://gw.example.com/coding/v1';

describe('floydCodeFeedbackUploadUrl', () => {
  it('uses the feedback upload_url path on the configured base URL', () => {
    vi.stubEnv('FLOYD_CODE_BASE_URL', CONFIGURED_BASE_URL);
    expect(floydCodeFeedbackUploadUrl()).toBe(`${CONFIGURED_BASE_URL}/feedback/upload_url`);
    expect(floydCodeFeedbackUploadUrl('https://api.example/coding/v1///')).toBe(
      'https://api.example/coding/v1/feedback/upload_url',
    );
  });

  it('resolves to empty when no base URL is configured', () => {
    vi.stubEnv('FLOYD_CODE_BASE_URL', undefined);
    expect(floydCodeFeedbackUploadUrl()).toBe('');
    expect(floydCodeFeedbackUploadUrl('')).toBe('');
  });
});

describe('floydCodeFeedbackUploadCompleteUrl', () => {
  it('uses the feedback upload_complete path on the configured base URL', () => {
    vi.stubEnv('FLOYD_CODE_BASE_URL', CONFIGURED_BASE_URL);
    expect(floydCodeFeedbackUploadCompleteUrl()).toBe(
      `${CONFIGURED_BASE_URL}/feedback/upload_complete`,
    );
    expect(floydCodeFeedbackUploadCompleteUrl('https://api.example/coding/v1///')).toBe(
      'https://api.example/coding/v1/feedback/upload_complete',
    );
  });

  it('resolves to empty when no base URL is configured', () => {
    vi.stubEnv('FLOYD_CODE_BASE_URL', undefined);
    expect(floydCodeFeedbackUploadCompleteUrl()).toBe('');
    expect(floydCodeFeedbackUploadCompleteUrl('')).toBe('');
  });
});

describe('fetchCreateFeedbackUploadUrl', () => {
  it('POSTs JSON body with bearer auth and parses upload parts', async () => {
    const fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({
          code: 0,
          upload: {
            id: 28,
            upload_id: 'tos-multipart-id',
            part_size: 8,
            total_parts: 1,
            parts: [
              { part_number: 1, url: 'https://example.test/part1', method: 'PUT', size: 123 },
            ],
          },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchCreateFeedbackUploadUrl('access-token', SAMPLE_BODY, {
      baseUrl: CONFIGURED_BASE_URL,
    });

    expect(result).toEqual({
      kind: 'ok',
      upload_id: 28,
      parts: [{ part_number: 1, url: 'https://example.test/part1', method: 'PUT', size: 123 }],
    });

    const calls = fetchMock.mock.calls as unknown as [string, RequestInit?][];
    const [calledUrl, init] = calls[0]!;
    expect(calledUrl).toBe(`${CONFIGURED_BASE_URL}/feedback/upload_url`);
    expect(init?.method).toBe('POST');

    const headers = new Headers((init?.headers ?? {}) as Record<string, string>);
    expect(headers.get('authorization')).toBe('Bearer access-token');
    expect(JSON.parse(init?.body as string)).toEqual(SAMPLE_BODY);
  });

  it('returns an error when the response omits parts', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify({ code: 0, upload: { id: 28 } }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    );

    const result = await fetchCreateFeedbackUploadUrl('access-token', SAMPLE_BODY, {
      baseUrl: CONFIGURED_BASE_URL,
    });

    expect(result).toEqual({
      kind: 'error',
      message: 'Feedback upload request failed: missing upload id or parts.',
    });
  });

  it('returns an error when a part is missing required fields', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(
          JSON.stringify({
            code: 0,
            upload: { id: 28, parts: [{ part_number: 1 }] },
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    );

    const result = await fetchCreateFeedbackUploadUrl('access-token', SAMPLE_BODY, {
      baseUrl: CONFIGURED_BASE_URL,
    });

    expect(result).toEqual({
      kind: 'error',
      message: 'Feedback upload request failed: missing upload id or parts.',
    });
  });

  it('returns an error with status when the server responds 401', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('', { status: 401 })));

    const result = await fetchCreateFeedbackUploadUrl('access-token', SAMPLE_BODY, {
      baseUrl: CONFIGURED_BASE_URL,
    });

    expect(result.kind).toBe('error');
    if (result.kind !== 'error') return;
    expect(result.status).toBe(401);
    expect(result.message).toMatch(/401/);
  });
});

describe('fetchCompleteFeedbackUpload', () => {
  it('POSTs upload_id and parts with bearer auth', async () => {
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchCompleteFeedbackUpload(
      'access-token',
      {
        upload_id: 28,
        parts: [
          { part_number: 1, etag: '"etag-1"' },
          { part_number: 2, etag: '"etag-2"' },
        ],
      },
      { baseUrl: CONFIGURED_BASE_URL },
    );

    expect(result).toEqual({ kind: 'ok' });
    const calls = fetchMock.mock.calls as unknown as [string, RequestInit?][];
    const [calledUrl, init] = calls[0]!;
    expect(calledUrl).toBe(`${CONFIGURED_BASE_URL}/feedback/upload_complete`);
    expect(JSON.parse(init?.body as string)).toEqual({
      upload_id: 28,
      parts: [
        { part_number: 1, etag: '"etag-1"' },
        { part_number: 2, etag: '"etag-2"' },
      ],
    });
  });
});

describe('unconfigured base URL', () => {
  it('does not request the upload_url endpoint', async () => {
    vi.stubEnv('FLOYD_CODE_BASE_URL', undefined);
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchCreateFeedbackUploadUrl('access-token', SAMPLE_BODY);

    expect(result.kind).toBe('error');
    if (result.kind !== 'error') return;
    expect(result.message).toBe(
      'Feedback upload request failed: no managed base URL is configured. Set FLOYD_CODE_BASE_URL.',
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('does not request the upload_complete endpoint', async () => {
    vi.stubEnv('FLOYD_CODE_BASE_URL', undefined);
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchCompleteFeedbackUpload('access-token', { upload_id: 28, parts: [] });

    expect(result.kind).toBe('error');
    if (result.kind !== 'error') return;
    expect(result.message).toBe(
      'Feedback upload request failed: no managed base URL is configured. Set FLOYD_CODE_BASE_URL.',
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
