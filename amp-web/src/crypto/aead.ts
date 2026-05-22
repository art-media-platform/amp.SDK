/**
 * AEAD + HKDF primitives shared across CryptoKit implementations.
 *
 * Mirrors `safe.SealAEAD` / `safe.OpenAEAD` / `safe.DeriveKey` in
 * amp.SDK/stdlib/safe/crypto.go.  All registered kits use XChaCha20-Poly1305
 * for symmetric AEAD and HKDF-SHA256 for key derivation.
 */

import { xchacha20poly1305 } from '@noble/ciphers/chacha';
import { randomBytes } from '@noble/ciphers/webcrypto';
import { hkdf } from '@noble/hashes/hkdf';
import { sha256 } from '@noble/hashes/sha256';

/** Output length of derived symmetric keys (256-bit). */
export const DEKSize = 32;

/** XChaCha20-Poly1305 nonce length (192-bit). */
export const NonceSize = 24;

/** HKDF-SHA256 → 32-byte derived key. */
export function deriveKey(rootKey: Uint8Array, salt: Uint8Array | null, info: Uint8Array): Uint8Array {
  return hkdf(sha256, rootKey, salt ?? new Uint8Array(0), info, DEKSize);
}

/**
 * Seal `plaintext` under `key` with optional `aad`.  Returns nonce +
 * ciphertext (the AEAD tag is appended to the ciphertext by @noble/ciphers).
 */
export function sealAEAD(
  key: Uint8Array,
  plaintext: Uint8Array,
  aad?: Uint8Array,
): { nonce: Uint8Array; ciphertext: Uint8Array } {
  if (key.length !== DEKSize) {
    throw new Error(`safe: key must be ${DEKSize} bytes, got ${key.length}`);
  }
  const nonce = randomBytes(NonceSize);
  const cipher = xchacha20poly1305(key, nonce, aad);
  const ciphertext = cipher.encrypt(plaintext);
  return { nonce, ciphertext };
}

/** Open AEAD ciphertext.  Throws on tag mismatch (bad key, tamper, or wrong AAD). */
export function openAEAD(
  key: Uint8Array,
  nonce: Uint8Array,
  ciphertext: Uint8Array,
  aad?: Uint8Array,
): Uint8Array {
  if (key.length !== DEKSize) {
    throw new Error(`safe: key must be ${DEKSize} bytes, got ${key.length}`);
  }
  if (nonce.length !== NonceSize) {
    throw new Error(`safe: nonce must be ${NonceSize} bytes, got ${nonce.length}`);
  }
  const cipher = xchacha20poly1305(key, nonce, aad);
  return cipher.decrypt(ciphertext);
}

/** Concatenate byte arrays into a fresh buffer. */
export function concat(...parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((n, p) => n + p.length, 0);
  const out = new Uint8Array(total);
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}

/** Lexicographic compare; returns <0, 0, or >0. */
export function bytesCompare(a: Uint8Array, b: Uint8Array): number {
  const n = Math.min(a.length, b.length);
  for (let ii = 0; ii < n; ii++) {
    if (a[ii] !== b[ii]) return a[ii] - b[ii];
  }
  return a.length - b.length;
}
