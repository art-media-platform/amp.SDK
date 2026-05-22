/**
 * Crypto entry point — kit registry + session-bound AmpCrypto.
 *
 * Poly25519 is registered eagerly (default kit, bundled).  P-256 and
 * secp256k1 lazy-load on first use per AUDIT §2.10 — they're absent from
 * the default bundle so cards/widgets that only need BYOK stay small.
 */

import { poly25519KitOps } from './poly25519';
import {
  type AmpCrypto,
  CryptoKitID,
  type KeyPair,
  type KitOps,
  type PubKeyRef,
} from './types';

const kits = new Map<CryptoKitID, KitOps>([[CryptoKitID.Poly25519, poly25519KitOps]]);

/**
 * Resolve a kit by ID.  Throws for unregistered kits with a pointer to the
 * lazy loader — P-256 and secp256k1 land when their auth schemes ship
 * (YubiKey, crypto-wallet).
 */
export async function getKit(kit: CryptoKitID): Promise<KitOps> {
  const ops = kits.get(kit);
  if (ops) return ops;
  switch (kit) {
    case CryptoKitID.P256:
      throw new Error('CryptoKit P-256 not yet bundled (lazy-load lands with YubiKey login)');
    case CryptoKitID.Secp256k1:
      throw new Error('CryptoKit secp256k1 not yet bundled (lazy-load lands with crypto-wallet seal)');
    default:
      throw new Error(`unknown CryptoKitID ${kit}`);
  }
}

/** Internal hook for the lazy loaders to register themselves. */
export function registerKit(ops: KitOps): void {
  kits.set(ops.kit, ops);
}

/** Direct kit-dispatched seal — used when the consumer knows the recipient's kit + pub. */
export async function seal(plaintext: Uint8Array, recipient: PubKeyRef): Promise<Uint8Array> {
  const ops = await getKit(recipient.kit);
  return ops.seal(plaintext, recipient.pub);
}

/** Direct kit-dispatched open — used when the consumer holds the keypair off-adapter. */
export async function open(sealed: Uint8Array, recipient: KeyPair): Promise<Uint8Array> {
  const ops = await getKit(recipient.kit);
  return ops.open(sealed, recipient.prv);
}

/** Construct a session-bound AmpCrypto.  The adapter installs the keypair on login. */
export function createAmpCrypto(): AmpCrypto {
  let keyPair: KeyPair | null = null;
  return {
    setEncryptKey(kp) {
      if (keyPair && keyPair.prv !== kp?.prv) {
        keyPair.prv.fill(0);
      }
      keyPair = kp;
    },
    async seal(plaintext) {
      if (!keyPair) throw new Error('seal: no EncryptKey installed (no active session)');
      const ops = await getKit(keyPair.kit);
      return ops.seal(plaintext, keyPair.pub);
    },
    async open(sealed) {
      if (!keyPair) throw new Error('open: no EncryptKey installed (no active session)');
      const ops = await getKit(keyPair.kit);
      return ops.open(sealed, keyPair.prv);
    },
    getEncryptPub() {
      if (!keyPair) return null;
      return { kit: keyPair.kit, pub: keyPair.pub };
    },
  };
}

export { CryptoKitID } from './types';
export type { AmpCrypto, KeyPair, KitOps, PubKeyRef } from './types';
export { base64ToBytes, bytesToBase64 } from './base64';
