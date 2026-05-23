import { useCallback } from 'react';
import { useAmpContext } from '../provider.js';

/**
 * useAmpCrypto — sealed-box BYOK helpers bound to the session member.
 *
 * `seal(plaintext)` returns anonymous-sender HPKE bytes safe to upsert into a
 * channel item; `open(sealed)` recovers plaintext using the session's
 * EncryptKey.  The adapter installs that key on login (device-local, generated
 * on first use), so seal/open work for any logged-in member; both throw if
 * there is no active session.  See SKILL-amp-web-SDK.md §"Storing
 * user-supplied secrets (BYOK)" for the canonical usage pattern.
 */
export function useAmpCrypto(): {
  seal: (plaintext: Uint8Array) => Promise<Uint8Array>;
  open: (sealed: Uint8Array) => Promise<Uint8Array>;
} {
  const { adapter } = useAmpContext();

  const seal = useCallback((plaintext: Uint8Array) => adapter.seal(plaintext), [adapter]);
  const open = useCallback((sealed: Uint8Array) => adapter.open(sealed), [adapter]);

  return { seal, open };
}
