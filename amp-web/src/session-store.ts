/**
 * Durable session store — the member survives a page reload.
 *
 * Persists the LoginResponse triple (SessionToken + ExpiresAt + Member) keyed
 * by vault URL; AmpWebClient.restoreSession() reads it back on a fresh load
 * and re-validates against GET /api/v1/session before trusting it.  The token
 * is an opaque Bearer readable by any same-origin script — see
 * SECURITY-amp-web-SDK.md ("Session persistence") for the exposure this
 * implies and why it is accepted.
 *
 * No key material lives here: the device EncryptKey has its own store
 * (crypto/keystore.ts); this record is the Bearer + the member facts needed
 * to seed auth state before the host re-validates.
 */

import { SESSIONS_STORE, openAmpDB } from './idb.js';
import type { AmpMember } from './types.js';

/** The persisted session record — the LoginResponse triple (wire casing). */
export interface StoredSession {
  SessionToken: string;
  ExpiresAt: number;      // unix seconds
  Member: AmpMember;
}

/**
 * SessionStore persists one session per vault URL.  The default browser
 * implementation is IndexedDB-backed; Node / SSR contexts fall back to an
 * in-memory store.  Consumers may inject their own (e.g. an OS keychain
 * bridge) via AmpWebClientOpts.sessionStore.
 */
export interface SessionStore {
  /** Return the vault's stored session, or null if none is held. */
  load(vaultUrl: string): Promise<StoredSession | null>;
  /** Persist the vault's session, overwriting any prior entry. */
  save(vaultUrl: string, session: StoredSession): Promise<void>;
  /** Drop the vault's stored session (no-op when none is held). */
  clear(vaultUrl: string): Promise<void>;
}

/** IndexedDB-backed storage — the browser default (shared amp-web DB, idb.ts). */
export class IndexedDBSessionStore implements SessionStore {
  async load(vaultUrl: string): Promise<StoredSession | null> {
    const db = await openAmpDB();
    return new Promise<StoredSession | null>((resolve, reject) => {
      const tx = db.transaction(SESSIONS_STORE, 'readonly');
      const req = tx.objectStore(SESSIONS_STORE).get(vaultUrl);
      req.onsuccess = () => resolve((req.result as StoredSession | undefined) ?? null);
      req.onerror = () => reject(req.error ?? new Error('amp-web: indexedDB get failed'));
    });
  }

  async save(vaultUrl: string, session: StoredSession): Promise<void> {
    const db = await openAmpDB();
    return new Promise<void>((resolve, reject) => {
      const tx = db.transaction(SESSIONS_STORE, 'readwrite');
      tx.objectStore(SESSIONS_STORE).put(session, vaultUrl);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error ?? new Error('amp-web: indexedDB put failed'));
    });
  }

  async clear(vaultUrl: string): Promise<void> {
    const db = await openAmpDB();
    return new Promise<void>((resolve, reject) => {
      const tx = db.transaction(SESSIONS_STORE, 'readwrite');
      tx.objectStore(SESSIONS_STORE).delete(vaultUrl);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error ?? new Error('amp-web: indexedDB delete failed'));
    });
  }
}

/**
 * In-memory storage — the fallback when IndexedDB is unavailable (Node, SSR,
 * tests).  Lives for the process lifetime only; nothing persists to disk.
 */
export class MemorySessionStore implements SessionStore {
  private sessions = new Map<string, StoredSession>();

  async load(vaultUrl: string): Promise<StoredSession | null> {
    return this.sessions.get(vaultUrl) ?? null;
  }

  async save(vaultUrl: string, session: StoredSession): Promise<void> {
    this.sessions.set(vaultUrl, session);
  }

  async clear(vaultUrl: string): Promise<void> {
    this.sessions.delete(vaultUrl);
  }
}

/**
 * defaultSessionStore picks IndexedDB in the browser and an in-memory store
 * everywhere else.  The in-memory fallback keeps login working in Node and
 * SSR without a reload-survival guarantee (there is no reload to survive).
 */
export function defaultSessionStore(): SessionStore {
  if (typeof indexedDB !== 'undefined') {
    return new IndexedDBSessionStore();
  }
  return new MemorySessionStore();
}
