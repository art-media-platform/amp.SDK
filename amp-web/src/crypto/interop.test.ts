/**
 * Go ↔ TypeScript Poly25519 interop.
 *
 * Both this suite and stdlib/safe/poly25519/poly25519_interop_test.go open the
 * SAME two sealed blobs against the SAME fixed X25519 private scalar — one blob
 * produced by the Go kit, one by this TS kit.  If both sides open both blobs,
 * the seal/open envelope is byte-compatible in both directions.  Regenerate
 * the vectors only if the envelope or KDF info string changes.
 */

import { describe, expect, it } from 'vitest';
import { hexToBytes } from '@noble/hashes/utils';

import { open } from './index.js';
import { CryptoKitID, type KeyPair } from './types.js';

const dec = new TextDecoder();

const PRV = hexToBytes('0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20');
const PUB = hexToBytes('07a37cbc142093c8b755dc1b10e86cb426374ad16aa853ed0bdfc0b2b86d1c7c');

const GO_SEALED = hexToBytes(
  '5d9e6b07e0dcada3d8e474be492e3e016c7c369fa8c3782d3967a08b43c30c75f02277bc4cb038b585c0a7a0eece081' +
    '96cfe2c301018ad10959de095f2dca80032f6af8cf615346d456482b17f2edcc2809909d1cbaa7c31e6d1f671df60bb' +
    '259f31331d5f8cebdb66a73cde4dd67273398f6b162bbf3c9514078de5',
);
const TS_SEALED = hexToBytes(
  'bdcb79760c8bd1ed1ff4c697ee5924a0ac455edcc63c497995174d6783ba6f3b4d6c32dc0fae18c3902644f8425c516' +
    '71ef20086cd226771a009609de6231c8c19678dc466fc9c3c910efe8fdd3c1aca42503295082365ab828e0a62ba671b' +
    '97aa8479d441590af302f34735c5e81a21f2767bfb0cb026a828bd2769',
);

const recipient: KeyPair = { kit: CryptoKitID.Poly25519, pub: PUB, prv: PRV };

describe('Go↔TS Poly25519 interop', () => {
  it('opens a Go-sealed vector', async () => {
    const opened = await open(GO_SEALED, recipient);
    expect(dec.decode(opened)).toBe('amp Poly25519 interop: sealed by Go, opened anywhere');
  });

  it('opens a TS-sealed vector', async () => {
    const opened = await open(TS_SEALED, recipient);
    expect(dec.decode(opened)).toBe('amp Poly25519 interop: sealed by TS, opened anywhere');
  });
});
