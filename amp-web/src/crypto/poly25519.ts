/**
 * Poly25519 kit — X25519 ECDH + HKDF-SHA256 + XChaCha20-Poly1305.
 *
 * Byte-compatible with amp.SDK/stdlib/safe/poly25519/poly25519.go.  A payload
 * sealed here opens in Go; a payload sealed in Go opens here.
 *
 * Envelope: eph_pub (32) || nonce (24) || ciphertext+tag
 * KDF info: "safe.X25519." || min(ephPub,peerPub) || max(ephPub,peerPub)
 */

import { x25519 } from '@noble/curves/ed25519';
import { randomBytes } from '@noble/ciphers/webcrypto';

import { CryptoKitID, type KeyPair, type KitOps } from './types';
import { NonceSize, bytesCompare, concat, deriveKey, openAEAD, sealAEAD } from './aead';

/** X25519 public key size (also private key size). */
export const X25519KeySize = 32;

const KDFInfoPrefix = new TextEncoder().encode('safe.X25519.');

function deriveSharedKey(prv: Uint8Array, peerPub: Uint8Array, myPub: Uint8Array): Uint8Array {
  const shared = x25519.getSharedSecret(prv, peerPub);
  try {
    const [lo, hi] = bytesCompare(myPub, peerPub) <= 0 ? [myPub, peerPub] : [peerPub, myPub];
    const info = concat(KDFInfoPrefix, lo, hi);
    return deriveKey(shared, null, info);
  } finally {
    shared.fill(0);
  }
}

/** Seal `plaintext` for the recipient under HPKE base mode (anonymous sender). */
async function seal(plaintext: Uint8Array, peerPub: Uint8Array): Promise<Uint8Array> {
  if (peerPub.length !== X25519KeySize) {
    throw new Error(`poly25519: peer pub must be ${X25519KeySize} bytes, got ${peerPub.length}`);
  }
  const ephPrv = randomBytes(X25519KeySize);
  try {
    const ephPub = x25519.getPublicKey(ephPrv);
    const shared = deriveSharedKey(ephPrv, peerPub, ephPub);
    try {
      const { nonce, ciphertext } = sealAEAD(shared, plaintext);
      return concat(ephPub, nonce, ciphertext);
    } finally {
      shared.fill(0);
    }
  } finally {
    ephPrv.fill(0);
  }
}

/** Open a sealed buffer using the recipient's prv key. */
async function open(sealed: Uint8Array, prv: Uint8Array): Promise<Uint8Array> {
  if (prv.length !== X25519KeySize) {
    throw new Error(`poly25519: prv key must be ${X25519KeySize} bytes, got ${prv.length}`);
  }
  if (sealed.length < X25519KeySize + NonceSize) {
    throw new Error('poly25519: ciphertext too short');
  }
  const ephPub = sealed.subarray(0, X25519KeySize);
  const nonce = sealed.subarray(X25519KeySize, X25519KeySize + NonceSize);
  const ct = sealed.subarray(X25519KeySize + NonceSize);

  const myPub = x25519.getPublicKey(prv);
  const shared = deriveSharedKey(prv, ephPub, myPub);
  try {
    return openAEAD(shared, nonce, ct);
  } finally {
    shared.fill(0);
  }
}

async function generateKeyPair(): Promise<KeyPair> {
  const prv = randomBytes(X25519KeySize);
  const pub = x25519.getPublicKey(prv);
  return { kit: CryptoKitID.Poly25519, pub, prv };
}

export const poly25519KitOps: KitOps = {
  kit: CryptoKitID.Poly25519,
  seal,
  open,
  generateKeyPair,
};
