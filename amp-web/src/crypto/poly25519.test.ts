/**
 * Round-trip tests for the Poly25519 kit.
 *
 * Covers AUDIT-maplable §4.5 milestone: seal → (channel-item round-trip
 * simulated by a value store) → open → assert plaintext matches.  Also
 * pressure-tests the contract: wrong key fails, tampered ciphertext fails,
 * empty plaintext round-trips, envelope sizes hit the documented layout.
 */

import { describe, expect, it } from 'vitest';

import { NonceSize } from './aead.js';
import { createAmpCrypto, getKit, open, seal } from './index.js';
import { X25519KeySize, poly25519KitOps } from './poly25519.js';
import { CryptoKitID, type KeyPair } from './types.js';

const enc = new TextEncoder();
const dec = new TextDecoder();

async function freshKeyPair(): Promise<KeyPair> {
  return poly25519KitOps.generateKeyPair();
}

describe('poly25519 kit', () => {
  it('round-trips plaintext via seal/open against a fresh keypair', async () => {
    const kp = await freshKeyPair();
    const plaintext = enc.encode('cesium ion live key — never the wire plaintext');

    const sealed = await poly25519KitOps.seal(plaintext, kp.pub);
    const opened = await poly25519KitOps.open(sealed, kp.prv);

    expect(dec.decode(opened)).toBe(dec.decode(plaintext));
  });

  it('envelope layout is eph_pub (32) || nonce (24) || ciphertext+tag', async () => {
    const kp = await freshKeyPair();
    const plaintext = enc.encode('x');
    const sealed = await poly25519KitOps.seal(plaintext, kp.pub);

    // Header is the documented size; ciphertext is plaintext.length + 16-byte AEAD tag.
    expect(sealed.length).toBe(X25519KeySize + NonceSize + plaintext.length + 16);
  });

  it('rejects a non-recipient prv key', async () => {
    const bob = await freshKeyPair();
    const eve = await freshKeyPair();
    const sealed = await poly25519KitOps.seal(enc.encode('for bob'), bob.pub);

    await expect(poly25519KitOps.open(sealed, eve.prv)).rejects.toThrow();
  });

  it('rejects a tampered ciphertext', async () => {
    const kp = await freshKeyPair();
    const sealed = await poly25519KitOps.seal(enc.encode('untouched'), kp.pub);
    sealed[sealed.length - 1] ^= 0x01;

    await expect(poly25519KitOps.open(sealed, kp.prv)).rejects.toThrow();
  });

  it('produces a different sealed payload each call (fresh ephemeral key)', async () => {
    const kp = await freshKeyPair();
    const plaintext = enc.encode('same plaintext');

    const a = await poly25519KitOps.seal(plaintext, kp.pub);
    const b = await poly25519KitOps.seal(plaintext, kp.pub);

    expect(a).not.toEqual(b);
  });

  it('round-trips empty plaintext', async () => {
    const kp = await freshKeyPair();
    const sealed = await poly25519KitOps.seal(new Uint8Array(0), kp.pub);
    const opened = await poly25519KitOps.open(sealed, kp.prv);

    expect(opened.length).toBe(0);
  });

  it('round-trips a 4 KiB payload (BYOK upper bound)', async () => {
    const kp = await freshKeyPair();
    const plaintext = new Uint8Array(4096);
    for (let ii = 0; ii < plaintext.length; ii++) plaintext[ii] = ii & 0xff;

    const sealed = await poly25519KitOps.seal(plaintext, kp.pub);
    const opened = await poly25519KitOps.open(sealed, kp.prv);

    expect(opened).toEqual(plaintext);
  });

  it('rejects a malformed peer pub key', async () => {
    const plaintext = enc.encode('whatever');
    await expect(poly25519KitOps.seal(plaintext, new Uint8Array(31))).rejects.toThrow();
  });

  it('rejects a malformed prv key on open', async () => {
    const kp = await freshKeyPair();
    const sealed = await poly25519KitOps.seal(enc.encode('whatever'), kp.pub);
    await expect(poly25519KitOps.open(sealed, new Uint8Array(31))).rejects.toThrow();
  });

  it('rejects a sealed buffer shorter than the header', async () => {
    const kp = await freshKeyPair();
    await expect(poly25519KitOps.open(new Uint8Array(10), kp.prv)).rejects.toThrow(/too short/);
  });
});

describe('kit registry', () => {
  it('exposes Poly25519 by default', async () => {
    const ops = await getKit(CryptoKitID.Poly25519);
    expect(ops.kit).toBe(CryptoKitID.Poly25519);
  });

  it('reports lazy-load pointers for un-bundled kits', async () => {
    await expect(getKit(CryptoKitID.P256)).rejects.toThrow(/P-256/);
    await expect(getKit(CryptoKitID.Secp256k1)).rejects.toThrow(/secp256k1/);
  });

  it('seal/open helpers dispatch to the recipient kit', async () => {
    const kp = await freshKeyPair();
    const plaintext = enc.encode('dispatched');
    const sealed = await seal(plaintext, { kit: kp.kit, pub: kp.pub });
    const opened = await open(sealed, kp);
    expect(dec.decode(opened)).toBe('dispatched');
  });
});

describe('AmpCrypto session surface', () => {
  it('seals/opens through the installed session keypair', async () => {
    const crypto = createAmpCrypto();
    const session = await freshKeyPair();
    crypto.setEncryptKey(session);

    const plaintext = enc.encode('openrouter-***');
    const sealed = await crypto.seal(plaintext);
    const opened = await crypto.open(sealed);

    expect(dec.decode(opened)).toBe(dec.decode(plaintext));
  });

  it('simulates the SKILL §6.2 BYOK round-trip: seal → upsert → read → open', async () => {
    // Stand-in for the channel item store — the upsert path is opaque to the
    // wire layer, so a Map<itemID, value> is a faithful mock of the contract.
    const itemStore = new Map<string, { cesium: Uint8Array }>();
    const memberID = 'member-uid';

    const crypto = createAmpCrypto();
    const session = await freshKeyPair();
    crypto.setEncryptKey(session);

    const cesiumKey = enc.encode('CESIUM-ION-LIVE-TOKEN');

    // Save: seal then upsert.
    const sealed = await crypto.seal(cesiumKey);
    itemStore.set(memberID, { cesium: sealed });

    // Read back: query item, open.
    const stored = itemStore.get(memberID);
    expect(stored).toBeDefined();
    const opened = await crypto.open(stored!.cesium);

    expect(dec.decode(opened)).toBe('CESIUM-ION-LIVE-TOKEN');
  });

  it('throws when no session is installed', async () => {
    const crypto = createAmpCrypto();
    await expect(crypto.seal(enc.encode('nope'))).rejects.toThrow(/no EncryptKey/);
    await expect(crypto.open(new Uint8Array(64))).rejects.toThrow(/no EncryptKey/);
  });

  it('clears keypair on logout (setEncryptKey(null))', async () => {
    const crypto = createAmpCrypto();
    crypto.setEncryptKey(await freshKeyPair());
    expect(crypto.getEncryptPub()).not.toBeNull();

    crypto.setEncryptKey(null);
    expect(crypto.getEncryptPub()).toBeNull();
    await expect(crypto.seal(enc.encode('after logout'))).rejects.toThrow(/no EncryptKey/);
  });
});
