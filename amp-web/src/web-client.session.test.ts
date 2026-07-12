/**
 * Session persistence: login persists the session, a fresh client on the same
 * store rehydrates it without re-login (the same-device reload), and the
 * session drops everywhere — memory + store — on logout, 401, and expiry.
 *
 * Network is stubbed (fetch + WebSocket); the stores are real.
 */

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { MemoryKeyStorage } from './crypto/keystore.js';
import { AmpError } from './errors.js';
import { MemorySessionStore } from './session-store.js';
import { AmpWebClient } from './web-client.js';

const MEMBER = 'member-uid-xyz';
const TOKEN = 'tok-1';
const VAULT = 'http://127.0.0.1:5193';

const realFetch = globalThis.fetch;
const realWebSocket = (globalThis as { WebSocket?: unknown }).WebSocket;

/** Inert WebSocket so connectWs() neither opens a socket nor schedules reconnects. */
class FakeWebSocket {
  static OPEN = 1;
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((e: unknown) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(_url: string) {}
  send(): void {}
  close(): void {}
}

/** Per-test wire log + tunable /session behavior. */
let calls: { path: string; auth: string | null }[] = [];
let sessionStatus = 200;      // GET /session responds with this
let expiresAt = 0;            // ExpiresAt stamped on login + session responses

function stubFetch(): void {
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const path = url.replace(/^.*\/api\/v1/, '');
    const headers = (init?.headers ?? {}) as Record<string, string>;
    calls.push({ path, auth: headers['Authorization'] ?? null });

    if (path === '/login') {
      return new Response(
        JSON.stringify({
          SessionToken: TOKEN,
          ExpiresAt: expiresAt,
          Member: { ID: MEMBER, PlanetID: 'planet', DisplayName: 'Xyz' },
        }),
        { status: 200, headers: { 'content-type': 'application/json' } },
      );
    }
    if (path === '/session') {
      if (sessionStatus !== 200) {
        return new Response(
          JSON.stringify({ Code: 'AuthRequired', Message: 'no session' }),
          { status: sessionStatus, headers: { 'content-type': 'application/json' } },
        );
      }
      return new Response(
        JSON.stringify({
          Member: { ID: MEMBER, PlanetID: 'planet', DisplayName: 'Xyz' },
          ExpiresAt: expiresAt,
        }),
        { status: 200, headers: { 'content-type': 'application/json' } },
      );
    }
    if (path === '/me') {
      return new Response(
        JSON.stringify({ ID: MEMBER, PlanetID: 'planet', DisplayName: 'Xyz' }),
        { status: 200, headers: { 'content-type': 'application/json' } },
      );
    }
    if (path === '/logout') {
      return new Response(null, { status: 204 });
    }
    if (path.startsWith('/channels/expired')) {
      return new Response(
        JSON.stringify({ Code: 'AuthRequired', Message: 'session expired' }),
        { status: 401, headers: { 'content-type': 'application/json' } },
      );
    }
    return new Response('not found', { status: 404 });
  }) as typeof fetch;
}

beforeEach(() => {
  calls = [];
  sessionStatus = 200;
  expiresAt = 0;
  stubFetch();
  (globalThis as { WebSocket?: unknown }).WebSocket = FakeWebSocket as unknown;
});

afterEach(() => {
  globalThis.fetch = realFetch;
  (globalThis as { WebSocket?: unknown }).WebSocket = realWebSocket;
});

function newAdapter(store: MemorySessionStore): AmpWebClient {
  return new AmpWebClient({
    vaultUrl: VAULT,
    planetTag: 'planet',
    encryptKeyStorage: new MemoryKeyStorage(),
    sessionStore: store,
  });
}

describe('session persistence across reload', () => {
  it('login persists; a fresh client on the same store rehydrates without re-login', async () => {
    const store = new MemorySessionStore();

    const first = newAdapter(store);
    await first.login({ Scheme: 'memberToken', MemberToken: 'whatever' });
    expect((await store.load(VAULT))?.SessionToken).toBe(TOKEN);

    // "Reload": a brand-new client sharing only the persisted stores.
    const second = newAdapter(store);
    expect(second.getSession()).toBeNull();

    const events: (string | null)[] = [];
    second.onAuthChange(m => events.push(m?.ID ?? null));
    const member = await second.restoreSession();

    expect(member?.ID).toBe(MEMBER);
    expect(second.getSession()?.ID).toBe(MEMBER);
    expect(events).toEqual([MEMBER]);
    // Rehydration validated against /session with the stored Bearer — no /login.
    const sessionCalls = calls.filter(c => c.path === '/session');
    expect(sessionCalls).toHaveLength(1);
    expect(sessionCalls[0].auth).toBe(`Bearer ${TOKEN}`);
    expect(calls.filter(c => c.path === '/login')).toHaveLength(1); // the first client's only
  });

  it('restore installs the device EncryptKey — BYOK works after reload', async () => {
    const keyStore = new MemoryKeyStorage();
    const sessions = new MemorySessionStore();
    const opts = { vaultUrl: VAULT, planetTag: 'planet', encryptKeyStorage: keyStore, sessionStore: sessions };

    const first = new AmpWebClient(opts);
    await first.login({ Scheme: 'memberToken', MemberToken: 'whatever' });
    const sealed = await first.seal(new TextEncoder().encode('SURVIVES-RELOAD'));

    const second = new AmpWebClient(opts);
    await second.restoreSession();
    const opened = await second.open(sealed);
    expect(new TextDecoder().decode(opened)).toBe('SURVIVES-RELOAD');
  });

  it('restoreSession is a no-network null when nothing is stored', async () => {
    const amp = newAdapter(new MemorySessionStore());
    expect(await amp.restoreSession()).toBeNull();
    expect(calls).toHaveLength(0);
  });

  it('an expired stored session clears locally without a wire call', async () => {
    const store = new MemorySessionStore();
    expiresAt = Math.floor(Date.now() / 1000) - 60;   // already expired

    await newAdapter(store).login({ Scheme: 'memberToken', MemberToken: 'whatever' });
    calls = [];

    const second = newAdapter(store);
    expect(await second.restoreSession()).toBeNull();
    expect(calls).toHaveLength(0);
    expect(await store.load(VAULT)).toBeNull();
  });

  it('a host-rejected token (401) clears the stored session', async () => {
    const store = new MemorySessionStore();
    await newAdapter(store).login({ Scheme: 'memberToken', MemberToken: 'whatever' });

    sessionStatus = 401;
    const second = newAdapter(store);
    expect(await second.restoreSession()).toBeNull();
    expect(second.getSession()).toBeNull();
    expect(await store.load(VAULT)).toBeNull();
  });

  it('a transport failure leaves the stored session for the next attempt', async () => {
    const store = new MemorySessionStore();
    await newAdapter(store).login({ Scheme: 'memberToken', MemberToken: 'whatever' });

    globalThis.fetch = (async () => { throw new TypeError('network down'); }) as typeof fetch;
    const second = newAdapter(store);
    await expect(second.restoreSession()).rejects.toThrow('network down');
    expect(second.getSession()).toBeNull();
    expect((await store.load(VAULT))?.SessionToken).toBe(TOKEN);

    // Network back: the same client restores from the kept record.
    stubFetch();
    expect((await second.restoreSession())?.ID).toBe(MEMBER);
  });

  it('concurrent restores share one in-flight validation', async () => {
    const store = new MemorySessionStore();
    await newAdapter(store).login({ Scheme: 'memberToken', MemberToken: 'whatever' });
    calls = [];

    const second = newAdapter(store);
    const [a, b] = await Promise.all([second.restoreSession(), second.restoreSession()]);
    expect(a?.ID).toBe(MEMBER);
    expect(b?.ID).toBe(MEMBER);
    expect(calls.filter(c => c.path === '/session')).toHaveLength(1);
  });
});

describe('session drop paths', () => {
  it('logout clears the persisted session — a reload stays signed out', async () => {
    const store = new MemorySessionStore();
    const amp = newAdapter(store);
    await amp.login({ Scheme: 'memberToken', MemberToken: 'whatever' });
    await amp.logout();

    expect(await store.load(VAULT)).toBeNull();
    expect(await newAdapter(store).restoreSession()).toBeNull();
  });

  it('a 401 on any authed call drops the session everywhere', async () => {
    const store = new MemorySessionStore();
    const amp = newAdapter(store);
    await amp.login({ Scheme: 'memberToken', MemberToken: 'whatever' });

    const events: (string | null)[] = [];
    amp.onAuthChange(m => events.push(m?.ID ?? null));

    await expect(amp.query('expired', 'attr')).rejects.toMatchObject({
      status: 401,
      code: 'AuthRequired',
    });
    expect(amp.getSession()).toBeNull();
    expect(events).toEqual([null]);
    expect(await store.load(VAULT)).toBeNull();
  });

  it('a 401 from a failed re-login attempt does NOT clobber the live session', async () => {
    const store = new MemorySessionStore();
    const amp = newAdapter(store);
    await amp.login({ Scheme: 'memberToken', MemberToken: 'whatever' });

    globalThis.fetch = (async (input: RequestInfo | URL) => {
      if (String(input).endsWith('/api/v1/login')) {
        return new Response(
          JSON.stringify({ Code: 'AuthFailed', Message: 'bad credentials' }),
          { status: 401, headers: { 'content-type': 'application/json' } },
        );
      }
      return new Response('not found', { status: 404 });
    }) as typeof fetch;

    await expect(amp.login({ Scheme: 'memberToken', MemberToken: 'bad' }))
      .rejects.toBeInstanceOf(AmpError);
    expect(amp.getSession()?.ID).toBe(MEMBER);
    expect((await store.load(VAULT))?.SessionToken).toBe(TOKEN);
  });
});

describe('session wire methods', () => {
  it('fetchSession returns the host-validated Member + ExpiresAt', async () => {
    expiresAt = 4102444800;
    const amp = newAdapter(new MemorySessionStore());
    await amp.login({ Scheme: 'memberToken', MemberToken: 'whatever' });

    const sess = await amp.fetchSession();
    expect(sess.Member.ID).toBe(MEMBER);
    expect(sess.ExpiresAt).toBe(4102444800);
  });

  it('me returns the member record; both throw typed AmpError when signed out', async () => {
    const amp = newAdapter(new MemorySessionStore());
    await amp.login({ Scheme: 'memberToken', MemberToken: 'whatever' });
    expect((await amp.me()).ID).toBe(MEMBER);

    sessionStatus = 401;
    const anon = newAdapter(new MemorySessionStore());
    await expect(anon.fetchSession()).rejects.toBeInstanceOf(AmpError);
  });
});
