/**
 * Email-flow client methods: recoverEmail / redeemEmail / claimAccount against
 * a stubbed wire, plus the wallet-only degradation when the host has no email
 * credential store (501 Unsupported must leave wallet login fully usable).
 *
 * Network is stubbed (fetch + WebSocket); the stores are real.
 */

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { MemoryKeyStorage } from './crypto/keystore.js';
import { AmpError, AmpErrorCode } from './errors.js';
import { MemorySessionStore } from './session-store.js';
import { AmpWebClient } from './web-client.js';

const MEMBER = 'email-member-uid';
const TOKEN = 'tok-email-1';
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

/** Per-test wire log + tunable email-surface behavior. */
let calls: { path: string; auth: string | null; body: Record<string, unknown> }[] = [];
let emailStoreConfigured = true;   // false → email endpoints 501 like a host with no credential store
let claimStatus = 200;             // /account/claim responds with this when the store is configured

function loginResponseBody(): string {
  return JSON.stringify({
    SessionToken: TOKEN,
    ExpiresAt: 0,
    Member: { ID: MEMBER, PlanetID: 'planet', Email: 'alice@example.com' },
  });
}

function stubFetch(): void {
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const path = url.replace(/^.*\/api\/v1/, '');
    const headers = (init?.headers ?? {}) as Record<string, string>;
    const body = init?.body ? JSON.parse(String(init.body)) : {};
    calls.push({ path, auth: headers['Authorization'] ?? null, body });

    if (path === '/login/email/recover' || path === '/login/email/redeem') {
      if (!emailStoreConfigured) {
        return new Response(
          JSON.stringify({ Code: 'Unsupported', Message: 'credential store not configured' }),
          { status: 501, headers: { 'content-type': 'application/json' } },
        );
      }
      if (path === '/login/email/recover') {
        return new Response(
          JSON.stringify({ Status: 'accepted' }),
          { status: 202, headers: { 'content-type': 'application/json' } },
        );
      }
      return new Response(loginResponseBody(), {
        status: 200, headers: { 'content-type': 'application/json' },
      });
    }
    if (path === '/account/claim') {
      if (claimStatus !== 200) {
        return new Response(
          JSON.stringify({ Code: claimStatus === 409 ? 'Conflict' : 'AuthFailed', Message: 'refused' }),
          { status: claimStatus, headers: { 'content-type': 'application/json' } },
        );
      }
      return new Response(loginResponseBody(), {
        status: 200, headers: { 'content-type': 'application/json' },
      });
    }
    if (path === '/login') {
      return new Response(loginResponseBody(), {
        status: 200, headers: { 'content-type': 'application/json' },
      });
    }
    if (path === '/me') {
      return new Response(
        JSON.stringify({ ID: MEMBER, PlanetID: 'planet' }),
        { status: 200, headers: { 'content-type': 'application/json' } },
      );
    }
    return new Response('not found', { status: 404 });
  }) as typeof fetch;
}

beforeEach(() => {
  calls = [];
  emailStoreConfigured = true;
  claimStatus = 200;
  stubFetch();
  (globalThis as { WebSocket?: unknown }).WebSocket = FakeWebSocket as unknown;
});

afterEach(() => {
  globalThis.fetch = realFetch;
  (globalThis as { WebSocket?: unknown }).WebSocket = realWebSocket;
});

function newAdapter(store = new MemorySessionStore()): AmpWebClient {
  return new AmpWebClient({
    vaultUrl: VAULT,
    planetTag: 'planet',
    encryptKeyStorage: new MemoryKeyStorage(),
    sessionStore: store,
  });
}

describe('recoverEmail', () => {
  it('POSTs the email and resolves on the uniform 202', async () => {
    await newAdapter().recoverEmail('alice@example.com');
    expect(calls).toHaveLength(1);
    expect(calls[0].path).toBe('/login/email/recover');
    expect(calls[0].body).toEqual({ Email: 'alice@example.com' });
  });
});

describe('redeemEmail', () => {
  it('sends Token + NewPassword and installs the minted session', async () => {
    const store = new MemorySessionStore();
    const amp = newAdapter(store);
    const events: (string | null)[] = [];
    amp.onAuthChange(m => events.push(m?.ID ?? null));

    const member = await amp.redeemEmail({ token: 'recover-token-3f6b', newPassword: 'rotated-secret-4' });

    expect(calls[0].path).toBe('/login/email/redeem');
    expect(calls[0].body).toEqual({ Token: 'recover-token-3f6b', NewPassword: 'rotated-secret-4' });
    expect(member.ID).toBe(MEMBER);
    expect(amp.getSession()?.ID).toBe(MEMBER);
    expect(events).toEqual([MEMBER]);
    expect((await store.load(VAULT))?.SessionToken).toBe(TOKEN);

    // The minted Bearer rides subsequent calls — the client-side /me vantage.
    expect((await amp.me()).ID).toBe(MEMBER);
    expect(calls[calls.length - 1].auth).toBe(`Bearer ${TOKEN}`);
  });

  it('a bad code is a typed AmpError and no session is installed', async () => {
    globalThis.fetch = (async () => new Response(
      JSON.stringify({ Code: 'AuthFailed', Message: 'recovery token invalid or expired' }),
      { status: 401, headers: { 'content-type': 'application/json' } },
    )) as typeof fetch;

    const amp = newAdapter();
    await expect(amp.redeemEmail({ token: 'nope', newPassword: 'x' }))
      .rejects.toMatchObject({ status: 401, code: AmpErrorCode.AuthFailed });
    expect(amp.getSession()).toBeNull();
  });
});

describe('claimAccount', () => {
  it('sends Email + Token + NewPassword and installs the minted session', async () => {
    const amp = newAdapter();
    const member = await amp.claimAccount({
      email: 'carol@example.com',
      token: 'GEZDGNBV.GM4TSOJR',
      newPassword: 'first-secret-7',
    });

    expect(calls[0].path).toBe('/account/claim');
    expect(calls[0].body).toEqual({
      Email: 'carol@example.com',
      Token: 'GEZDGNBV.GM4TSOJR',
      NewPassword: 'first-secret-7',
    });
    expect(member.ID).toBe(MEMBER);
    expect(amp.getSession()?.ID).toBe(MEMBER);
  });

  it('an already-claimed account surfaces as AmpError Conflict (409)', async () => {
    claimStatus = 409;
    await expect(newAdapter().claimAccount({ email: 'c@x.com', token: 't', newPassword: 'p' }))
      .rejects.toMatchObject({ status: 409, code: AmpErrorCode.Conflict });
  });

  it('a 401 from a failed claim does NOT clobber the live session', async () => {
    const amp = newAdapter();
    await amp.login({ Scheme: 'memberToken', MemberToken: 'whatever' });

    claimStatus = 401;
    await expect(amp.claimAccount({ email: 'c@x.com', token: 'stale', newPassword: 'p' }))
      .rejects.toBeInstanceOf(AmpError);
    expect(amp.getSession()?.ID).toBe(MEMBER);
  });
});

describe('wallet-only degradation (email transport off)', () => {
  it('email endpoints throw typed Unsupported; wallet login still completes', async () => {
    emailStoreConfigured = false;
    const amp = newAdapter();

    // The UI's capability probe: a 501 carries the stable Unsupported code so
    // the login screen hides the email form instead of dead-ending the user.
    await expect(amp.recoverEmail('alice@example.com'))
      .rejects.toMatchObject({ status: 501, code: AmpErrorCode.Unsupported });
    await expect(amp.redeemEmail({ token: 't', newPassword: 'p' }))
      .rejects.toMatchObject({ status: 501, code: AmpErrorCode.Unsupported });

    // The non-email path stays fully usable on the same client instance.
    const member = await amp.login({ Scheme: 'memberToken', MemberToken: 'sso' });
    expect(member.ID).toBe(MEMBER);
    expect(amp.getSession()?.ID).toBe(MEMBER);
  });
});
