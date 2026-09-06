/**
 * The op batch as POSTed to /tx: TxOp.BaseEdit (the displayed _EditID,
 * base32) is emitted into the op value as BaseEdit_0/BaseEdit_1 — exact
 * uint64 integer literals, the forums.Post fields 7/8 the host consumes as
 * the edit's base — and the SDK-side BaseEdit key itself never reaches the
 * wire.  Asserted on the request BODY TEXT, since a JS number cannot carry
 * the digits.
 */

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { MemoryKeyStorage } from './crypto/keystore.js';
import { Attr } from './generated/amp.std.consts.js';
import { MemorySessionStore } from './session-store.js';
import { AmpWebClient, encodeWireJSON } from './web-client.js';

const realFetch = globalThis.fetch;
let bodies: string[] = [];

beforeEach(() => {
  bodies = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input).replace(/^.*\/api\/v1/, '');
    if (path === '/tx') {
      bodies.push(String(init?.body));
      return new Response(JSON.stringify({ TxID: 't', Results: [{ ItemID: 'p', EditID: 'e' }] }), {
        status: 200, headers: { 'content-type': 'application/json' },
      });
    }
    return new Response('{}', { status: 404 });
  }) as typeof fetch;
});
afterEach(() => { globalThis.fetch = realFetch; });

function client(): AmpWebClient {
  return new AmpWebClient({
    vaultUrl: 'http://127.0.0.1:5193',
    planetTag: 'home',
    encryptKeyStorage: new MemoryKeyStorage(),
    sessionStore: new MemorySessionStore(),
  });
}

const BASE = '4zs-80kzpjzhjx-53bkpndft1-m95'; // == Attr.App.id, a golden the Go side renders identically
const [HI, LO] = Attr.App.id;

describe('encodeWireJSON', () => {
  it('emits bigint values as bare integer literals with every digit', () => {
    expect(encodeWireJSON({ A: 2n ** 64n - 1n, B: 'x', C: [1n, 2] })).toBe('{"A":18446744073709551615,"B":"x","C":[1,2]}');
  });
});

describe('edit-verb payload rail', () => {
  it('invoke() folds TxOp.BaseEdit into the value as BaseEdit_0/_1 exact uint64 literals', async () => {
    await client().invoke('amp://~/forums/moderate', [
      { Kind: 'upsert', Channel: 'topic', Attr: 'forums.Post', ItemID: 'p', Value: { Status: 1 }, BaseEdit: BASE },
    ]);
    expect(bodies).toHaveLength(1);
    const body = bodies[0];
    expect(body).toContain(`"BaseEdit_0":${HI.toString()},"BaseEdit_1":${LO.toString()}`);
    expect(body).toContain('"Status":1');
    expect(body).not.toContain('"BaseEdit":');
    expect(body).toContain('"InvokeURL":"amp://~/forums/moderate"');
    // The literal survives as the exact digits, never a rounded double.
    expect(HI > BigInt(Number.MAX_SAFE_INTEGER)).toBe(true);
    expect(body).not.toContain(String(Number(HI)));
  });

  it('tx() without BaseEdit emits no BaseEdit keys', async () => {
    await client().tx([{ Kind: 'upsert', Channel: 'c', Attr: 'a', ItemID: 'i', Value: { X: 1 } }]);
    expect(bodies[0]).not.toContain('BaseEdit');
    expect(JSON.parse(bodies[0]).Ops[0].Value).toEqual({ X: 1 });
  });
});
