/**
 * resolveMedia(): the URI the server answers is the carrier — a member
 * resolve's /www/{UID}.{ext}?t=<media token> (app.www www.media_auth.go) is
 * the only credential the media plane accepts — so the client hands it
 * through verbatim, query string and all, and builds no /www/ URL of its
 * own.  The mocked fetch is the /media/resolve door: Bearer-gated, answering
 * the posted Tag with a token URI.
 */

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { AmpError, AmpErrorCode } from './errors.js';
import { MemoryKeyStorage } from './crypto/keystore.js';
import { MemorySessionStore, type StoredSession } from './session-store.js';
import { AmpWebClient } from './web-client.js';

const VAULT = 'http://127.0.0.1:5193';
const UID = '06efv-w28sb-60036d-tm6dt-m6dtm';
// A packed 3-UID Address (channel, attr, item): the BlobRef cell a sealed blob
// resolves by (the amp/webapi testdata/media.json byCell golden).
const CELL = 'z41s2udrrbg06q1qry3f1rvg7vtt1jcwuz0pqj8my77ry1yrn13q4e67n47xrnyf2cy29fze1me36';
const TOKEN = 'GE3TEMBQGAYDAMBQ.6yxaa-aaaaa-abbbbb-bbbbb-bcccc.06efv-w28sb-60036d-tm6dt-m6dtm.GA.MFRGGZDFMZTWQ2LK';
const realFetch = globalThis.fetch;

interface ResolveCall { path: string; auth: string | null; body: unknown }
let calls: ResolveCall[] = [];
let revoked = false;

function stubFetch(): void {
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input).replace(VAULT, '');
    const headers = (init?.headers ?? {}) as Record<string, string>;
    const body = init?.body ? JSON.parse(String(init.body)) : null;
    calls.push({ path, auth: headers['Authorization'] ?? null, body });
    if (path === '/api/v1/session') {
      return new Response(JSON.stringify({ Member: { ID: UID, PlanetID: 'home', DisplayName: 'Alice' }, ExpiresAt: 0 }), {
        status: 200, headers: { 'content-type': 'application/json' },
      });
    }
    if (path.startsWith(`/api/v1/channels/uploads%2F${UID}/attrs/amp.blob.BlobRef/items`)) {
      return new Response(JSON.stringify({
        Items: [{
          _ItemID: UID, _EditID: UID, _FromID: UID, _UpdatedAt: '2026-10-17T00:00:00Z',
          Value: { BlobTag: { UID } }, _Address: CELL,
        }],
        HasMore: false,
      }), { status: 200, headers: { 'content-type': 'application/json' } });
    }
    if (path !== '/api/v1/media/resolve') {
      return new Response('not found', { status: 404 });
    }
    if (revoked || !headers['Authorization']) {
      return new Response(JSON.stringify({ Code: 'AuthRequired', Message: 'missing or expired session' }), {
        status: 401, headers: { 'content-type': 'application/json' },
      });
    }
    const blob = body.Blob as { UID: string; ContentTypeRaw?: string };
    const subtype = (blob.ContentTypeRaw || 'text/plain').toLowerCase().split('/')[1];
    return new Response(JSON.stringify({ ...blob, URI: `/www/${blob.UID}.${subtype}?t=${TOKEN}` }), {
      status: 200, headers: { 'content-type': 'application/json' },
    });
  }) as typeof fetch;
}

async function loggedInClient(): Promise<{ client: AmpWebClient; store: MemorySessionStore }> {
  const store = new MemorySessionStore();
  const client = new AmpWebClient({
    vaultUrl: VAULT,
    planetTag: 'home',
    encryptKeyStorage: new MemoryKeyStorage(),
    sessionStore: store,
  });
  // Seed the persisted session the way a prior login left it; restore
  // re-validates it against GET /session (stubbed 200).
  await store.save(VAULT, {
    SessionToken: 'bearer-alice',
    ExpiresAt: 0,
    Member: { ID: UID, PlanetID: 'home', DisplayName: 'Alice' } as unknown as StoredSession['Member'],
  });
  await client.restoreSession();
  return { client, store };
}

describe('resolveMedia carries the server URI verbatim', () => {
  beforeEach(() => { calls = []; revoked = false; stubFetch(); });
  afterEach(() => { globalThis.fetch = realFetch; });

  it('answers the token URL as given — path, query and token untouched', async () => {
    const { client } = await loggedInClient();
    const out = await client.resolveMedia({ UID, ContentTypeRaw: 'video/mp4', I: 10, Units: 2 });
    expect(out.URI).toBe(`/www/${UID}.mp4?t=${TOKEN}`);
    expect(out.UID).toBe(UID);
    const resolves = calls.filter(c => c.path === '/api/v1/media/resolve');
    expect(resolves).toHaveLength(1);
    expect(resolves[0]!.auth).toBe('Bearer bearer-alice');
    expect(resolves[0]!.body).toEqual({ Blob: { UID, ContentTypeRaw: 'video/mp4', I: 10, Units: 2 }, PlanetTag: 'home' });
  });

  it('sends the cell as Cell when given, and no Cell key when it is empty (Tag-only)', async () => {
    const { client } = await loggedInClient();
    await client.resolveMedia({ UID, ContentTypeRaw: 'video/mp4' }, undefined, CELL);
    await client.resolveMedia({ UID, ContentTypeRaw: 'video/mp4' }, undefined, '');
    await client.resolveMedia({ UID, ContentTypeRaw: 'video/mp4' });
    const bodies = calls.filter(c => c.path === '/api/v1/media/resolve').map(c => c.body as Record<string, unknown>);
    expect(bodies).toHaveLength(3);
    expect(bodies[0]!['Cell']).toBe(CELL);
    expect(bodies[0]).toEqual({ Blob: { UID, ContentTypeRaw: 'video/mp4' }, PlanetTag: 'home', Cell: CELL });
    expect('Cell' in bodies[1]!).toBe(false);
    expect('Cell' in bodies[2]!).toBe(false);
  });

  it('sends the BlobRef\'s own Cell by default, outside the posted Tag; an argument wins', async () => {
    const { client } = await loggedInClient();
    const own = { UID, ContentTypeRaw: 'audio/mpeg', Cell: CELL };
    await client.resolveMedia(own);
    const byOwn = calls[calls.length - 1]!.body as { Blob: Record<string, unknown>; Cell?: string };
    expect(byOwn.Cell).toBe(CELL);
    expect(byOwn.Blob).toEqual({ UID, ContentTypeRaw: 'audio/mpeg' });
    const other = CELL.replace(/^z/, 'y');
    await client.resolveMedia(own, undefined, other);
    expect((calls[calls.length - 1]!.body as { Cell?: string }).Cell).toBe(other);
  });

  it('a listed item carries its cell as _Address, the Cell a resolve names (no client-side packing)', async () => {
    const { client } = await loggedInClient();
    // The member's own uploads node: the '/' rides percent-encoded.
    const listed = await client.query<{ BlobTag: { UID: string } }>(`uploads/${UID}`, 'amp.blob.BlobRef');
    const list = calls.filter(c => c.path.startsWith('/api/v1/channels/')).pop()!;
    expect(list.path.startsWith(`/api/v1/channels/uploads%2F${UID}/attrs/`)).toBe(true);
    expect(listed.data).toHaveLength(1);
    expect(listed.data[0]!._Address).toBe(CELL);
    await client.resolveMedia({ UID, ContentTypeRaw: 'audio/mpeg' }, undefined, listed.data[0]!._Address);
    const resolve = calls.filter(c => c.path === '/api/v1/media/resolve').pop()!;
    expect((resolve.body as { Cell?: string }).Cell).toBe(CELL);
  });

  it('exposes no client-side /www/ URL builder', () => {
    const client = new AmpWebClient({
      vaultUrl: VAULT,
      planetTag: 'home',
      encryptKeyStorage: new MemoryKeyStorage(),
      sessionStore: new MemorySessionStore(),
    });
    expect((client as unknown as Record<string, unknown>)['mediaUrl']).toBeUndefined();
  });

  it('a revoked session fails the resolve as 401 and drops the stored session', async () => {
    const { client, store } = await loggedInClient();
    revoked = true;
    await expect(client.resolveMedia({ UID, ContentTypeRaw: 'video/mp4' })).rejects.toMatchObject({ code: AmpErrorCode.AuthRequired } satisfies Partial<AmpError>);
    expect(await store.load(VAULT)).toBeNull();
  });
});
