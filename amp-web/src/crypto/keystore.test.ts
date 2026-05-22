/**
 * Device-local EncryptKey store — persistence + same-device reopen.
 *
 * The store is what closes the BYOK "no wire delivers the EncryptKey" gap for
 * the same-device case: a member's key is generated once, persisted, and
 * re-used on every later session so previously sealed secrets reopen.
 */

import { describe, expect, it } from 'vitest';

import { open, seal } from './index';
import {
  MemoryKeyStorage,
  defaultEncryptKeyStorage,
  resolveDeviceEncryptKey,
} from './keystore';
import { CryptoKitID } from './types';

const enc = new TextEncoder();
const dec = new TextDecoder();
const MEMBER = 'member-uid-abc';

describe('EncryptKeyStorage', () => {
  it('returns null for an unknown member', async () => {
    const store = new MemoryKeyStorage();
    expect(await store.load(MEMBER)).toBeNull();
  });

  it('round-trips a saved keypair with detached buffers', async () => {
    const store = new MemoryKeyStorage();
    const kp = { kit: CryptoKitID.Poly25519, pub: new Uint8Array([1, 2, 3]), prv: new Uint8Array([4, 5, 6]) };
    await store.save(MEMBER, kp);

    const loaded = await store.load(MEMBER);
    expect(loaded).not.toBeNull();
    expect(loaded!.pub).toEqual(kp.pub);
    expect(loaded!.prv).toEqual(kp.prv);
    // Mutating the caller's buffer must not corrupt the stored copy.
    kp.prv.fill(0);
    const reloaded = await store.load(MEMBER);
    expect(reloaded!.prv).toEqual(new Uint8Array([4, 5, 6]));
  });

  it('defaults to in-memory storage when IndexedDB is absent (Node/SSR)', () => {
    expect(typeof indexedDB).toBe('undefined');
    expect(defaultEncryptKeyStorage()).toBeInstanceOf(MemoryKeyStorage);
  });
});

describe('resolveDeviceEncryptKey', () => {
  it('generates + persists a key on first use', async () => {
    const store = new MemoryKeyStorage();
    const kp = await resolveDeviceEncryptKey(store, MEMBER);

    expect(kp.kit).toBe(CryptoKitID.Poly25519);
    expect(kp.pub.length).toBe(32);
    expect(kp.prv.length).toBe(32);
    expect(await store.load(MEMBER)).not.toBeNull();
  });

  it('returns the SAME persisted key on a later session', async () => {
    const store = new MemoryKeyStorage();
    const first = await resolveDeviceEncryptKey(store, MEMBER);
    const second = await resolveDeviceEncryptKey(store, MEMBER);

    expect(second.pub).toEqual(first.pub);
    expect(second.prv).toEqual(first.prv);
  });

  it('keeps distinct keys per member', async () => {
    const store = new MemoryKeyStorage();
    const a = await resolveDeviceEncryptKey(store, 'member-a');
    const b = await resolveDeviceEncryptKey(store, 'member-b');
    expect(a.pub).not.toEqual(b.pub);
  });

  it('survives a returned key being zeroized (logout) — the store keeps its own copy', async () => {
    const store = new MemoryKeyStorage();
    const first = await resolveDeviceEncryptKey(store, MEMBER);
    first.prv.fill(0); // mimic crypto-surface zeroize on logout

    const again = await resolveDeviceEncryptKey(store, MEMBER);
    expect(again.prv.some(b => b !== 0)).toBe(true);
  });

  it('reopens secrets sealed in a prior session (same-device)', async () => {
    const store = new MemoryKeyStorage();
    const secret = enc.encode('CESIUM-ION-LIVE-TOKEN');

    // Session 1: resolve key, seal-to-self.
    const session1 = await resolveDeviceEncryptKey(store, MEMBER);
    const sealed = await seal(secret, { kit: session1.kit, pub: session1.pub });

    // Session 2 (e.g. a page reload): same storage → same key → open.
    const session2 = await resolveDeviceEncryptKey(store, MEMBER);
    const opened = await open(sealed, session2);

    expect(dec.decode(opened)).toBe('CESIUM-ION-LIVE-TOKEN');
  });
});
