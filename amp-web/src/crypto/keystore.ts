/**
 * Device-local EncryptKey store — the per-member sealed-box key, auto-managed.
 *
 * BYOK seal/open need the session member's EncryptKey installed.  Rather than
 * make every consumer install one out-of-band, the adapter resolves a
 * per-member key here on login: load it from device storage, or generate and
 * persist one on first use.  The private key never leaves the device, so a
 * seal-to-self stays opaque to the host and to every other planet member —
 * admin-can't-read holds for free (see SECURITY-amp-web-SDK.md).
 *
 * Scope is same-device.  The key is device-local, so a member who clears
 * browser storage or signs in on a second device re-derives a fresh key there
 * and re-enters their (re-enterable) BYOK secrets.  Cross-device recovery is a
 * separate, larger mechanism (a wrapped-key envelope the host stores opaquely)
 * and is intentionally not built here.
 */

import { ENCRYPT_KEYS_STORE, openAmpDB } from '../idb.js';
import { getKit } from './index.js';
import { CryptoKitID, type KeyPair } from './types.js';

/**
 * EncryptKeyStorage persists one keypair per member.  The default browser
 * implementation is IndexedDB-backed; Node / SSR contexts fall back to an
 * in-memory store.  Consumers may inject their own (e.g. an OS keychain
 * bridge) via AmpWebClientOpts.encryptKeyStorage.
 */
export interface EncryptKeyStorage {
  /** Return the member's stored keypair, or null if none is held yet. */
  load(memberID: string): Promise<KeyPair | null>;
  /** Persist the member's keypair, overwriting any prior entry. */
  save(memberID: string, keyPair: KeyPair): Promise<void>;
}

/**
 * cloneKeyPair returns a deep copy with fresh byte buffers.  The session
 * crypto surface zeroizes a key's private bytes when it is replaced or cleared
 * (logout), so a stored keypair must never share its buffers with the one
 * handed to the crypto surface — otherwise logout would wipe the persisted key.
 */
function cloneKeyPair(keyPair: KeyPair): KeyPair {
  return {
    kit: keyPair.kit,
    pub: new Uint8Array(keyPair.pub),
    prv: new Uint8Array(keyPair.prv),
  };
}

/** IndexedDB-backed storage — the browser default (shared amp-web DB, idb.ts). */
export class IndexedDBKeyStorage implements EncryptKeyStorage {
  async load(memberID: string): Promise<KeyPair | null> {
    const db = await openAmpDB();
    return new Promise<KeyPair | null>((resolve, reject) => {
      const tx = db.transaction(ENCRYPT_KEYS_STORE, 'readonly');
      const req = tx.objectStore(ENCRYPT_KEYS_STORE).get(memberID);
      req.onsuccess = () => {
        const rec = req.result as KeyPair | undefined;
        // Normalize to fresh Uint8Arrays — the structured-clone read already
        // detaches from any in-memory buffer, and this keeps the type exact.
        resolve(rec ? cloneKeyPair(rec) : null);
      };
      req.onerror = () => reject(req.error ?? new Error('amp-web: indexedDB get failed'));
    });
  }

  async save(memberID: string, keyPair: KeyPair): Promise<void> {
    const db = await openAmpDB();
    return new Promise<void>((resolve, reject) => {
      const tx = db.transaction(ENCRYPT_KEYS_STORE, 'readwrite');
      tx.objectStore(ENCRYPT_KEYS_STORE).put(cloneKeyPair(keyPair), memberID);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error ?? new Error('amp-web: indexedDB put failed'));
    });
  }
}

/**
 * In-memory storage — the fallback when IndexedDB is unavailable (Node, SSR,
 * tests).  Lives for the process lifetime only; nothing persists to disk.
 */
export class MemoryKeyStorage implements EncryptKeyStorage {
  private keys = new Map<string, KeyPair>();

  async load(memberID: string): Promise<KeyPair | null> {
    const keyPair = this.keys.get(memberID);
    return keyPair ? cloneKeyPair(keyPair) : null;
  }

  async save(memberID: string, keyPair: KeyPair): Promise<void> {
    this.keys.set(memberID, cloneKeyPair(keyPair));
  }
}

/**
 * defaultEncryptKeyStorage picks IndexedDB in the browser and an in-memory
 * store everywhere else.  The in-memory fallback keeps seal/open working in
 * Node and SSR without a persistence guarantee.
 */
export function defaultEncryptKeyStorage(): EncryptKeyStorage {
  if (typeof indexedDB !== 'undefined') {
    return new IndexedDBKeyStorage();
  }
  return new MemoryKeyStorage();
}

/**
 * resolveDeviceEncryptKey returns the member's device-local EncryptKey,
 * generating and persisting one on first use.  An existing key is returned
 * as-is regardless of its kit, so previously sealed data stays openable; the
 * `kit` argument governs only first-time generation (default Poly25519, the
 * bundled kit).
 */
export async function resolveDeviceEncryptKey(
  storage: EncryptKeyStorage,
  memberID: string,
  kit: CryptoKitID = CryptoKitID.Poly25519,
): Promise<KeyPair> {
  const existing = await storage.load(memberID);
  if (existing) {
    return existing;
  }
  const ops = await getKit(kit);
  const fresh = await ops.generateKeyPair();
  await storage.save(memberID, fresh);
  return fresh;
}
