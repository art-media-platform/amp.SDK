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
const UID = '06e-fvw28sb600-36dtm6dtm6-dtm';
const TOKEN = 'GE3TEMBQGAYDAMBQ.6yx-aaaaaaaaaa-bbbbbbbbbb-ccc.06e-fvw28sb600-36dtm6dtm6-dtm.GA.MFRGGZDFMZTWQ2LK';
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
