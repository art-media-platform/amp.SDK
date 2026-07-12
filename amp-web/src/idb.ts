/**
 * Shared IndexedDB handle for the SDK's device-local stores.
 *
 * One database ("amp-web") holds every object store the SDK persists — the
 * per-member EncryptKeys (crypto/keystore.ts) and the per-vault session
 * records (session-store.ts).  This module owns the database name + version,
 * so adding a store is a version bump here and nowhere else.
 */

const DB_NAME = 'amp-web';
const DB_VERSION = 2;

export const ENCRYPT_KEYS_STORE = 'encryptKeys';
export const SESSIONS_STORE = 'sessions';

const ALL_STORES = [ENCRYPT_KEYS_STORE, SESSIONS_STORE] as const;

let dbPromise: Promise<IDBDatabase> | null = null;

/** Open (or reuse) the shared amp-web database, creating missing stores. */
export function openAmpDB(): Promise<IDBDatabase> {
  if (dbPromise) {
    return dbPromise;
  }
  const promise = new Promise<IDBDatabase>((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      for (const store of ALL_STORES) {
        if (!db.objectStoreNames.contains(store)) {
          db.createObjectStore(store);
        }
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error ?? new Error('amp-web: indexedDB open failed'));
  });
  // Don't cache a permanently-rejected handle — a transient open failure
  // (private-mode quota, a blocked upgrade) must not disable device storage
  // for the tab's lifetime.  Clear it so the next call retries.
  promise.catch(() => {
    if (dbPromise === promise) {
      dbPromise = null;
    }
  });
  dbPromise = promise;
  return promise;
}
