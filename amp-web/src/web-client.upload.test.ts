/**
 * upload(): the single-POST /upload path and the sequential chunk door
 * (/upload/chunk).  The mocked fetch is a mini chunk server enforcing the
 * wire contract of app.www wire.api.upload.chunk.go — per-upload expected
 * index (409 on out-of-order/duplicate), 404 for an unknown uploadID, one
 * chunk in flight, 201 Tag on complete — so a client sequencing defect
 * surfaces as a refused chunk, not only as a failed hand-list assertion.
 */

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { AmpError, AmpErrorCode } from './errors.js';
import { MemoryKeyStorage } from './crypto/keystore.js';
import { MemorySessionStore } from './session-store.js';
import { AmpWebClient, DefaultUploadChunkBytes } from './web-client.js';

const VAULT = 'http://127.0.0.1:5193';
const TAG = {
  UID: '06e-fvw28sb600-36dtm6dtm6-dtm',
  URI: '/www/06efvw28sb60036dtm6dtm6dtm',
  ContentTypeRaw: 'video/mp4',
  I: 10,
  Units: 2,
};

const realFetch = globalThis.fetch;

interface ChunkCall {
  path: string;
  auth: string | null;
  fields: Record<string, string>;
  fileName: string;
  fileBytes: string;
}

let calls: ChunkCall[] = [];
/** Open uploads: uploadID → { nextIndex, received }. */
let open = new Map<string, { nextIndex: number; received: number }>();
/** Fault injection: a status to answer the chunk at this index with. */
let failIndex = -1;
let failStatus = 500;
/** Fault injection: acks name the wrong index. */
let misackIndex = false;

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function stubFetch(): void {
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input).replace(/^.*\/api\/v1/, '');
    const headers = (init?.headers ?? {}) as Record<string, string>;
    const form = init?.body as FormData;
    const fields: Record<string, string> = {};
    let fileName = '';
    let fileBytes = '';
    if (form instanceof FormData) {
      for (const [key, value] of form.entries()) {
        if (typeof value === 'string') fields[key] = value;
      }
      const part = form.get('file');
      if (part instanceof Blob) {
        fileName = (part as File).name ?? '';
        fileBytes = await part.text();
      }
    }
    calls.push({ path, auth: headers['Authorization'] ?? null, fields, fileName, fileBytes });

    if (path === '/login') {
      return json(200, { SessionToken: 'tok', ExpiresAt: 0, Member: { ID: 'm', PlanetID: 'p' } });
    }
    if (path === '/upload') {
      return json(201, TAG);
    }
    if (path !== '/upload/chunk') {
      return json(404, { Code: 'NotFound', Message: path });
    }

    // ── the chunk door contract ──
    const uploadID = fields['uploadID'];
    const index = Number(fields['index']);
    if (index === failIndex) {
      const code = { 401: 'AuthRequired', 409: 'Conflict' }[failStatus] ?? 'Internal';
      return json(failStatus, { Code: code, Message: 'injected' });
    }
    let upload = open.get(uploadID);
    if (index === 0) {
      if (upload) return json(409, { Code: 'Conflict', Message: 'duplicate index: upload already open' });
      upload = { nextIndex: 0, received: 0 };
      open.set(uploadID, upload);
    } else {
      if (!upload) return json(404, { Code: 'NotFound', Message: 'unknown uploadID' });
      if (index !== upload.nextIndex) {
        return json(409, { Code: 'Conflict', Message: `out-of-order index ${index}: expected ${upload.nextIndex}` });
      }
    }
    upload.nextIndex = index + 1;
    upload.received += fileBytes.length;
    if (fields['complete'] === '1') {
      open.delete(uploadID);
      return json(201, { ...TAG, I: upload.received });
    }
    return json(200, { uploadID, index: misackIndex ? index + 1 : index, received: upload.received });
  }) as typeof fetch;
}

function client(): AmpWebClient {
  return new AmpWebClient({
    vaultUrl: VAULT,
    planetTag: 'home',
    encryptKeyStorage: new MemoryKeyStorage(),
    sessionStore: new MemorySessionStore(),
  });
}

const TEN = new File(['abcdefghij'], 'clip.mp4');

beforeEach(() => {
  calls = [];
  open = new Map();
  failIndex = -1;
  failStatus = 500;
  misackIndex = false;
  stubFetch();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe('path selection', () => {
  it('a file within one chunk rides single-POST /upload', async () => {
    const ticks: number[] = [];
    const out = await client().upload(TEN, 'projects', { attr: 'media', onProgress: pct => ticks.push(pct) });
    expect(calls.map(c => c.path)).toEqual(['/upload']);
    expect(calls[0].fields).toEqual({ channel: 'projects', attr: 'media', planetTag: 'home' });
    expect(calls[0].fileBytes).toBe('abcdefghij');
    expect(ticks).toEqual([100]);
    expect(out).toEqual(TAG);
  });

  it('the default chunk size is the single-POST threshold', () => {
    expect(DefaultUploadChunkBytes).toBe(32 * 1024 * 1024);
  });

  it('a file larger than one chunk rides the chunk door (no explicit opt)', async () => {
    await client().upload(TEN, 'projects', { chunkSize: 4 });
    expect(calls.map(c => c.path)).toEqual(['/upload/chunk', '/upload/chunk', '/upload/chunk']);
  });

  it('chunked: false forces single-POST regardless of size', async () => {
    await client().upload(TEN, 'projects', { chunkSize: 4, chunked: false });
    expect(calls.map(c => c.path)).toEqual(['/upload']);
  });

  it('chunked: true on a one-chunk file seals on index 0', async () => {
    const ticks: number[] = [];
    const out = await client().upload(TEN, 'projects', { chunked: true, onProgress: pct => ticks.push(pct) });
    expect(calls.map(c => c.path)).toEqual(['/upload/chunk']);
    expect(calls[0].fields['index']).toBe('0');
    expect(calls[0].fields['complete']).toBe('1');
    expect(calls[0].fileBytes).toBe('abcdefghij');
    expect(ticks).toEqual([100]);
    expect(out.I).toBe(10);
  });
});

describe('chunk sequencing + completion', () => {
  it('sends strictly sequential chunks under one uploadID and seals on the last', async () => {
    const ticks: number[] = [];
    const out = await client().upload(TEN, 'projects', {
      chunkSize: 4,
      attr: 'media',
      metadata: { caption: 'x' },
      onProgress: pct => ticks.push(pct),
    });

    expect(calls.map(c => c.fields['index'])).toEqual(['0', '1', '2']);
    expect(calls.map(c => c.fileBytes)).toEqual(['abcd', 'efgh', 'ij']);
    const ids = new Set(calls.map(c => c.fields['uploadID']));
    expect(ids.size).toBe(1);
    const [uploadID] = ids;
    expect(uploadID.length).toBeGreaterThan(0);
    expect(uploadID.length).toBeLessThanOrEqual(128);

    // complete + the /upload vocabulary ride the sealing request only.
    expect(calls.slice(0, 2).every(c => c.fields['complete'] === undefined)).toBe(true);
    expect(calls.slice(0, 2).every(c => c.fields['planetTag'] === undefined)).toBe(true);
    expect(calls[2].fields).toMatchObject({
      complete: '1',
      channel: 'projects',
      attr: 'media',
      planetTag: 'home',
      metadata: '{"caption":"x"}',
    });
    // The file-part filename names the blob (ContentType inferred server-side).
    expect(calls.every(c => c.fileName === 'clip.mp4')).toBe(true);

    // One tick per ack from the server's cumulative count, then completion.
    expect(ticks).toEqual([40, 80, 100]);
    expect(out).toEqual({ ...TAG, I: 10 });
    expect(open.size).toBe(0);
  });

  it('explicit planetTag wins on the sealing request', async () => {
    await client().upload(TEN, 'projects', { chunkSize: 4, planetTag: 'other' });
    expect(calls[2].fields['planetTag']).toBe('other');
  });

  it('the Bearer rides every chunk', async () => {
    const amp = client();
    await amp.login({ Scheme: 'memberToken', MemberToken: 'whatever' });
    calls = [];
    await amp.upload(TEN, 'projects', { chunkSize: 4 });
    expect(calls.map(c => c.auth)).toEqual(['Bearer tok', 'Bearer tok', 'Bearer tok']);
  });

  it('two uploads never share an uploadID', async () => {
    const amp = client();
    await amp.upload(TEN, 'projects', { chunkSize: 4 });
    await amp.upload(TEN, 'projects', { chunkSize: 4 });
    expect(calls[0].fields['uploadID']).not.toBe(calls[3].fields['uploadID']);
  });

  it('the mock door refuses a replayed index (the guard the sequencing test leans on)', async () => {
    const form = new FormData();
    form.append('uploadID', 'u');
    form.append('index', '1');
    form.append('file', new Blob(['x']), 'x');
    const resp = await fetch(`${VAULT}/api/v1/upload/chunk`, { method: 'POST', body: form });
    expect(resp.status).toBe(404);
  });
});

describe('error paths', () => {
  it('a refused chunk throws the typed AmpError and stops the loop', async () => {
    failIndex = 1;
    failStatus = 409;
    const ticks: number[] = [];
    const err = await client()
      .upload(TEN, 'projects', { chunkSize: 4, onProgress: pct => ticks.push(pct) })
      .catch(e => e);
    expect(err).toBeInstanceOf(AmpError);
    expect((err as AmpError).status).toBe(409);
    expect((err as AmpError).code).toBe(AmpErrorCode.Conflict);
    expect(calls.length).toBe(2);       // chunk 0 acked, chunk 1 refused, nothing after
    expect(ticks).toEqual([40]);        // no completion tick
  });

  it('a failed sealing request throws (no Tag, no 100 tick)', async () => {
    failIndex = 2;
    failStatus = 500;
    const ticks: number[] = [];
    const err = await client()
      .upload(TEN, 'projects', { chunkSize: 4, onProgress: pct => ticks.push(pct) })
      .catch(e => e);
    expect(err).toBeInstanceOf(AmpError);
    expect((err as AmpError).status).toBe(500);
    expect(ticks).toEqual([40, 80]);
  });

  it('an ack naming the wrong chunk throws before the next chunk is sent', async () => {
    misackIndex = true;
    const err = await client().upload(TEN, 'projects', { chunkSize: 4 }).catch(e => e);
    expect(err).toBeInstanceOf(AmpError);
    expect((err as AmpError).code).toBe(AmpErrorCode.Conflict);
    expect((err as AmpError).message).toContain('ack names chunk 1');
    expect(calls.length).toBe(1);
  });

  it('a 401 mid-upload drops the local session (the apiFetch policy)', async () => {
    const amp = client();
    await amp.login({ Scheme: 'memberToken', MemberToken: 'whatever' });
    const authEvents: unknown[] = [];
    amp.onAuthChange(member => authEvents.push(member));
    failIndex = 1;
    failStatus = 401;
    const err = await amp.upload(TEN, 'projects', { chunkSize: 4 }).catch(e => e);
    expect((err as AmpError).status).toBe(401);
    expect(amp.getSession()).toBeNull();
    expect(authEvents).toEqual([null]);
  });
});
