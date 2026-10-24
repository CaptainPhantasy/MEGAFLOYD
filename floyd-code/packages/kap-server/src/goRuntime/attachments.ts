import { createReadStream, createWriteStream } from 'node:fs';
import { mkdir, readFile, rename, stat, writeFile } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';
import { basename, join, resolve } from 'node:path';
import { pipeline } from 'node:stream/promises';
import { Readable, Transform } from 'node:stream';
import multipart from '@fastify/multipart';
import type { FastifyInstance } from 'fastify';
import { fileMetaSchema, type FileMeta } from '@legacy-ai/agent-core-v2/app/file/fileService';
import { GoRequestError } from './client';
import { object, text } from './projection';
import type { GoRuntime } from './runtime';
import type { GoWorkspaces } from './workspaces';
import { parseRangeHeader } from '../lib/httpRange';
import { buildContentDisposition } from '../lib/contentDisposition';

const MAX_ATTACHMENT_BYTES = 128 * 1024 * 1024;
export interface GoAttachment { file_path: string; file_name: string; mime_type: string; content: string }

export class GoAttachments {
  constructor(readonly directory: string) {}

  private path(id: string): string {
    if (!/^f_[A-Za-z0-9_-]+$/.test(id)) throw new GoRequestError(400, 'Invalid attachment ID');
    return join(this.directory, id);
  }

  async save(source: NodeJS.ReadableStream, name: string, mediaType: string, expiresInSec?: number): Promise<FileMeta> {
    const id = `f_${randomUUID()}`;
    const root = this.path(id);
    await mkdir(root, { recursive: true });
    const filename = basename(name) || 'attachment';
    let size = 0;
    const limit = new Transform({ transform(chunk: Buffer, encoding, callback) { size += chunk.length; callback(size > MAX_ATTACHMENT_BYTES ? new GoRequestError(413, 'Attachment exceeds the 128 MiB upload limit') : null, chunk); } });
    try { await pipeline(source, limit, createWriteStream(join(root, 'data'), { flags: 'wx' })); }
    catch (error) { await rename(root, `${root}.failed-${Date.now()}`); throw error; }
    const meta: FileMeta = { id, name: filename, media_type: mediaType || 'application/octet-stream', size, created_at: new Date().toISOString(), expires_at: expiresInSec === undefined ? undefined : new Date(Date.now() + expiresInSec * 1000).toISOString() };
    await writeFile(join(root, 'metadata.json'), JSON.stringify(meta), 'utf8');
    return meta;
  }

  async get(id: string) {
    const root = this.path(id);
    let meta: FileMeta;
    try { meta = fileMetaSchema.parse(JSON.parse(await readFile(join(root, 'metadata.json'), 'utf8'))); }
    catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') throw new GoRequestError(404, 'Attachment was not found'); throw error; }
    if (meta.id !== id || basename(meta.name) !== meta.name) throw new Error('Invalid canonical attachment metadata');
    if (meta.expires_at && Date.parse(meta.expires_at) < Date.now()) throw new GoRequestError(410, 'This staged attachment has expired');
    return { meta, path: join(root, 'data') };
  }

  async remove(id: string): Promise<void> {
    await this.get(id);
    await rename(this.path(id), `${this.path(id)}.deleted-${Date.now()}`);
  }

  async materialize(runtime: GoRuntime, content: unknown[]): Promise<GoAttachment[]> {
    const result: GoAttachment[] = [];
    for (const value of content) {
      const part = object(value);
      if (part['type'] === 'text') continue;
      if (!['image', 'video', 'file'].includes(text(part['type']))) throw new GoRequestError(400, 'Only text, images, video and file attachments can be submitted');
      const source = object(part['source']);
      const id = text(part['file_id']) || text(source['file_id']);
      let bytes: Buffer;
      let name = text(part['name']);
      let mime = text(part['media_type']) || text(source['media_type']);
      if (id) {
        const file = await this.get(id);
        bytes = await readFile(file.path);
        name ||= file.meta.name;
        mime ||= file.meta.media_type;
      } else if (part['path'] || source['kind'] === 'path') {
        const path = resolve(runtime.cwd, text(part['path']) || text(source['path']));
        const details = await stat(path);
        if (!details.isFile() || details.size > MAX_ATTACHMENT_BYTES) throw new GoRequestError(400, 'Attachment path must be a file smaller than 128 MiB');
        bytes = await readFile(path);
        name ||= basename(path);
        mime ||= 'application/octet-stream';
      } else if (source['kind'] === 'base64') {
        bytes = Buffer.from(text(source['data']), 'base64');
      } else if (source['kind'] === 'url') {
        const url = text(source['url']);
        const data = /^data:([^;,]+);base64,(.*)$/s.exec(url);
        if (data) { bytes = Buffer.from(data[2]!, 'base64'); mime ||= data[1]!; }
        else {
          const address = new URL(url);
          if (!['http:', 'https:'].includes(address.protocol)) throw new GoRequestError(400, 'Attachment URL must use HTTP or HTTPS');
          const response = await fetch(address, { signal: AbortSignal.timeout(30_000) });
          if (!response.ok) throw new GoRequestError(400, `Attachment download returned HTTP ${response.status}`);
          if (Number(response.headers.get('content-length')) > MAX_ATTACHMENT_BYTES) throw new GoRequestError(413, 'Attachment exceeds 128 MiB');
          const chunks: Buffer[] = [];
          let size = 0;
          for await (const chunk of response.body ?? []) { const buffer = Buffer.from(chunk); size += buffer.length; if (size > MAX_ATTACHMENT_BYTES) throw new GoRequestError(413, 'Attachment exceeds 128 MiB'); chunks.push(buffer); }
          bytes = Buffer.concat(chunks);
          mime ||= response.headers.get('content-type')?.split(';')[0] || '';
          name ||= basename(address.pathname);
        }
      } else throw new GoRequestError(400, 'Attachment has no readable source');
      if (bytes.byteLength > MAX_ATTACHMENT_BYTES) throw new GoRequestError(413, 'Attachment exceeds 128 MiB');
      name = basename(name || `${part['type']}-${randomUUID()}`);
      mime ||= 'application/octet-stream';
      const target = new GoAttachments(join(runtime.dataDir, 'attachments'));
      const stored = await target.save(Readable.from(bytes), name, mime);
      result.push({ file_path: (await target.get(stored.id)).path, file_name: name, mime_type: mime, content: bytes.toString('base64') });
    }
    return result;
  }
}

export function registerGoAttachments(app: FastifyInstance, files: GoAttachments, workspaces: GoWorkspaces): void {
  app.register(multipart, { limits: { files: 1, fileSize: MAX_ATTACHMENT_BYTES } });
  app.post('/api/v1/files', async (req, reply) => {
    const part = await req.file();
    if (!part) throw new GoRequestError(400, 'Missing file upload');
    const fields = object(part.fields);
    const name = text(object(fields['name'])['value']) || part.filename;
    const expires = object(fields['expires_in_sec'])['value'];
    const seconds = expires === undefined ? undefined : Number(expires);
    if (seconds !== undefined && (!Number.isFinite(seconds) || seconds < 0)) throw new GoRequestError(400, 'Invalid attachment expiry');
    const data = await files.save(part.file, name, part.mimetype, seconds);
    return reply.send({ code: 0, msg: 'success', data, request_id: req.id });
  });
  app.delete<{ Params: { id: string } }>('/api/v1/files/:id', async (req) => { await files.remove(req.params.id); return { code: 0, msg: 'success', data: { deleted: true }, request_id: req.id }; });
  const download = async (req: { params: { id: string; sid?: string }; headers: Record<string, unknown> }, reply: import('fastify').FastifyReply) => {
    let store = files;
    if (req.params.sid) { const { runtime } = await workspaces.session(req.params.sid); store = new GoAttachments(join(runtime.dataDir, 'attachments')); }
    const file = await store.get(req.params.id);
    reply.type(file.meta.media_type).header('Content-Disposition', buildContentDisposition(file.meta.name, file.meta.media_type)).header('Accept-Ranges', 'bytes').header('ETag', `"${file.meta.id}-${file.meta.size}"`);
    const rangeHeader = typeof req.headers['range'] === 'string' ? req.headers['range'] : undefined;
    const range = parseRangeHeader(rangeHeader, file.meta.size);
    if (rangeHeader && !range) return reply.code(416).header('Content-Range', `bytes */${file.meta.size}`).send();
    if (range) return reply.code(206).header('Content-Length', range.length).header('Content-Range', `bytes ${range.start}-${range.end}/${file.meta.size}`).send(createReadStream(file.path, { start: range.start, end: range.end }));
    return reply.header('Content-Length', file.meta.size).send(createReadStream(file.path));
  };
  app.get<{ Params: { id: string } }>('/api/v1/files/:id', download);
  app.get<{ Params: { sid: string; id: string } }>('/api/v1/sessions/:sid/media/:id', download);
}
