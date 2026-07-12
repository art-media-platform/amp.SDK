/**
 * Constructor planetTag as the per-call default: it rides every REST call
 * (query, tx, invoke, single-op item verbs, upload, media resolve) via the
 * existing wire vocabulary (?planetTag= / body PlanetTag); an explicit
 * per-call tag wins; with neither, nothing is sent and the server resolves
 * the session's bound planet.  Client-side only — the server stays sole
 * authority (resolvePlanet + epoch/ACC gates).
 */

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { AmpWebClient } from './web-client.js';

const VAULT = 'http://127.0.0.1:5193';

const realFetch = globalThis.fetch;

let calls: { url: string; body: unknown }[] = [];

function stubFetch(): void {
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    let body: unknown = null;
    if (typeof init?.body === 'string') {
      body = JSON.parse(init.body);
    } else if (init?.body instanceof FormData) {
      body = init.body;
    }
    calls.push({ url, body });

    if (url.includes('/items')) {
      return new Response(
        JSON.stringify({ Items: [], HasMore: false }),
        { status: 200, headers: { 'content-type': 'application/json' } },
      );
    }
    return new Response(
      JSON.stringify({ TxID: 't', Results: [] }),
      { status: 200, headers: { 'content-type': 'application/json' } },
    );
  }) as typeof fetch;
}

function lastCall(): { url: string; body: unknown } {
  return calls[calls.length - 1];
}

function client(planetTag: string): AmpWebClient {
  return new AmpWebClient({ vaultUrl: VAULT, planetTag });
}

beforeEach(() => {
  calls = [];
  stubFetch();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

describe('constructor planetTag rides as the per-call default', () => {
  it('query (list) sends the constructor tag', async () => {
    await client('share-planet').query('ch', 'attr');
    expect(lastCall().url).toContain('planetTag=share-planet');
  });

  it('query (list): explicit per-call tag wins', async () => {
    await client('share-planet').query('ch', 'attr', { planetTag: 'other' });
    expect(lastCall().url).toContain('planetTag=other');
    expect(lastCall().url).not.toContain('share-planet');
  });

  it('query (single item) sends the constructor tag', async () => {
    await client('share-planet').query('ch', 'attr', { itemID: 'item-1' });
    expect(lastCall().url).toContain('planetTag=share-planet');
  });

  it('empty constructor tag sends nothing (session planet resolves server-side)', async () => {
    await client('').query('ch', 'attr');
    expect(lastCall().url).not.toContain('planetTag');
  });

  it('tx body carries the constructor tag; explicit wins; empty omits', async () => {
    const ops = [{ Kind: 'create' as const, Channel: 'ch', Attr: 'attr', Value: {} }];

    await client('share-planet').tx(ops);
    expect((lastCall().body as { PlanetTag?: string }).PlanetTag).toBe('share-planet');

    await client('share-planet').tx(ops, 'other');
    expect((lastCall().body as { PlanetTag?: string }).PlanetTag).toBe('other');

    await client('').tx(ops);
    expect('PlanetTag' in (lastCall().body as object)).toBe(false);
  });

  it('invoke body carries the constructor tag beside InvokeURL', async () => {
    const ops = [{ Kind: 'create' as const, Channel: 'ch', Attr: 'attr', Value: {} }];
    await client('share-planet').invoke('amp://~/forums/post', ops);
    const body = lastCall().body as { PlanetTag?: string; InvokeURL?: string };
    expect(body.PlanetTag).toBe('share-planet');
    expect(body.InvokeURL).toBe('amp://~/forums/post');
  });

  it('single-op item verbs ride ?planetTag=', async () => {
    const c = client('share-planet');
    await c.create('ch', 'attr', {});
    expect(lastCall().url).toContain('planetTag=share-planet');
    await c.upsert('ch', 'attr', 'item-1', {});
    expect(lastCall().url).toContain('planetTag=share-planet');
    await c.remove('ch', 'attr', 'item-1');
    expect(lastCall().url).toContain('planetTag=share-planet');
    await c.withdraw('ch', 'attr', 'item-1', { reason: 'Retracted' });
    expect(lastCall().url).toContain('/withdraw?planetTag=share-planet');
  });

  it('upload form + media resolve body carry the constructor tag', async () => {
    const c = client('share-planet');
    await c.upload(new File(['x'], 'x.txt'), 'ch');
    expect((lastCall().body as FormData).get('planetTag')).toBe('share-planet');

    await c.resolveMedia({ UID: 'blob-1' });
    expect((lastCall().body as { PlanetTag?: string }).PlanetTag).toBe('share-planet');
  });
});
