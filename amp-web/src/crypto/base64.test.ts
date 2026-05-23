/**
 * base64 helpers — the bridge that puts sealed bytes onto the JSON wire.
 *
 * The decisive property: a base64 string survives JSON.stringify round-trips
 * where a raw Uint8Array does not.  This is exactly the footgun the helpers
 * remove from the BYOK upsert/read path.
 */

import { describe, expect, it } from 'vitest';

import { base64ToBytes, bytesToBase64 } from './base64.js';
import { open, seal } from './index.js';
import { poly25519KitOps } from './poly25519.js';

const enc = new TextEncoder();
const dec = new TextDecoder();

describe('base64 wire helpers', () => {
  it('round-trips arbitrary bytes', () => {
    const bytes = new Uint8Array([0, 1, 2, 254, 255, 128, 64]);
    expect(base64ToBytes(bytesToBase64(bytes))).toEqual(bytes);
  });

  it('round-trips empty input', () => {
    expect(base64ToBytes(bytesToBase64(new Uint8Array(0)))).toEqual(new Uint8Array(0));
  });

  it('is stack-safe for large inputs (> chunk size)', () => {
    const big = new Uint8Array(100_000);
    for (let i = 0; i < big.length; i++) big[i] = i & 0xff;
    expect(base64ToBytes(bytesToBase64(big))).toEqual(big);
  });

  it('survives JSON.stringify where a raw Uint8Array does not', () => {
    const sealed = new Uint8Array([10, 20, 30, 40]);

    // The helper: encode → JSON → parse → decode recovers the bytes.
    const wire = JSON.parse(JSON.stringify({ cesium: bytesToBase64(sealed) }));
    expect(base64ToBytes(wire.cesium)).toEqual(sealed);

    // The footgun: a raw Uint8Array stringifies to an index-keyed object.
    const broken = JSON.parse(JSON.stringify({ cesium: sealed }));
    expect(broken.cesium).toEqual({ '0': 10, '1': 20, '2': 30, '3': 40 });
    expect(broken.cesium).not.toBeInstanceOf(Uint8Array);
  });

  it('carries a sealed BYOK secret through a JSON channel item', async () => {
    const kp = await poly25519KitOps.generateKeyPair();
    const secret = enc.encode('CESIUM-ION-LIVE-TOKEN');

    const sealedB64 = bytesToBase64(await seal(secret, { kit: kp.kit, pub: kp.pub }));
    const item = JSON.parse(JSON.stringify({ cesium: sealedB64 })); // wire round-trip
    const opened = await open(base64ToBytes(item.cesium), kp);

    expect(dec.decode(opened)).toBe('CESIUM-ION-LIVE-TOKEN');
  });
});
