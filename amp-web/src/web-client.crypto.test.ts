/**
 * Adapter-level BYOK: login auto-installs the device EncryptKey, so seal/open
 * work with no out-of-band setEncryptKey — the regression that closes the
 * "wire delivers no EncryptKey" gap for the same-device case.
 *
 * Network is stubbed (fetch + WebSocket); the crypto and keystore are real.
 */

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { MemoryKeyStorage } from './crypto/keystore.js';
import { AmpWebClient } from './web-client.js';

const enc = new TextEncoder();
const dec = new TextDecoder();
const MEMBER = 'member-uid-xyz';

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

beforeEach(() => {
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.endsWith('/api/v1/login')) {
      return new Response(
        JSON.stringify({ sessionToken: 'tok', expiresAt: 0, member: { id: MEMBER, planetID: 'planet' } }),
        { status: 200, headers: { 'content-type': 'application/json' } },
      );
    }
    if (url.endsWith('/api/v1/logout')) {
      return new Response(null, { status: 204 });
    }
    return new Response('not found', { status: 404 });
  }) as typeof fetch;
  (globalThis as { WebSocket?: unknown }).WebSocket = FakeWebSocket as unknown;
});

afterEach(() => {
  globalThis.fetch = realFetch;
  (globalThis as { WebSocket?: unknown }).WebSocket = realWebSocket;
});

function newAdapter(storage: MemoryKeyStorage): AmpWebClient {
  return new AmpWebClient({ vaultUrl: 'http://127.0.0.1:5193', planetTag: 'planet', encryptKeyStorage: storage });
}

describe('AmpWebClient BYOK auto-install', () => {
  it('seals/opens after login without an explicit setEncryptKey', async () => {
    const amp = newAdapter(new MemoryKeyStorage());
    await amp.login({ scheme: 'memberToken', memberToken: 'whatever' });

    const sealed = await amp.seal(enc.encode('OPENROUTER-LIVE'));
    const opened = await amp.open(sealed);

    expect(dec.decode(opened)).toBe('OPENROUTER-LIVE');
  });

  it('reopens prior-session secrets on a fresh adapter sharing storage (same-device reload)', async () => {
    const storage = new MemoryKeyStorage();

    const first = newAdapter(storage);
    await first.login({ scheme: 'memberToken', memberToken: 'whatever' });
    const sealed = await first.seal(enc.encode('CESIUM-ION-LIVE'));
    await first.logout();

    const second = newAdapter(storage);
    await second.login({ scheme: 'memberToken', memberToken: 'whatever' });
    const opened = await second.open(sealed);

    expect(dec.decode(opened)).toBe('CESIUM-ION-LIVE');
  });

  it('clears the key on logout — seal then throws', async () => {
    const amp = newAdapter(new MemoryKeyStorage());
    await amp.login({ scheme: 'memberToken', memberToken: 'whatever' });
    await amp.logout();

    await expect(amp.seal(enc.encode('after logout'))).rejects.toThrow(/no EncryptKey/);
  });
});
