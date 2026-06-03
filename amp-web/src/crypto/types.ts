/**
 * Crypto primitives for @art-media-platform/web — sealed-box BYOK helpers.
 *
 * Mirrors `safe.KitSpec.Encrypt.Seal/Open` in amp.SDK/stdlib/safe.  Byte-
 * compatible with the Go side: a payload sealed in TS opens cleanly Go-side
 * and vice versa.
 *
 * Default kit is Poly25519 (X25519 + XChaCha20-Poly1305 + HKDF-SHA256), pure
 * JS via @noble/curves + @noble/ciphers + @noble/hashes (no WASM).  P-256 and
 * secp256k1 kits are lazy-loaded — they don't enter the default bundle.
 */

/**
 * CryptoKitID is an SDK-local dispatch tag picking which kit implementation
 * seals/opens.  It never rides the wire — the seal envelope is
 * `eph_pub || nonce || ciphertext+tag` with no kit byte, and a key's kit is
 * known from its origin.  Go-side, kits are identified by a name-derived
 * tag.UID (safe.CryptoKit.Poly25519 / .P256 / .Secp256k1); these small
 * integers are the TS-internal equivalent, not those UIDs.
 */
export const CryptoKitID = {
  Unspecified: 0,
  Poly25519: 1,
  P256: 2,
  Secp256k1: 3,
} as const;

export type CryptoKitID = (typeof CryptoKitID)[keyof typeof CryptoKitID];

/** A raw keypair, kit-tagged.  `prv` is sensitive — keep in memory only. */
export interface KeyPair {
  kit: CryptoKitID;
  pub: Uint8Array;
  prv: Uint8Array;
}

/** A public-only key reference, for sealing to a recipient. */
export interface PubKeyRef {
  kit: CryptoKitID;
  pub: Uint8Array;
}

/**
 * KitOps is the pure-crypto surface registered per CryptoKitID.  Mirrors
 * `safe.EncryptOps` in Go.  Higher-level wrappers (client.seal / client.open)
 * dispatch through here based on the session member's kit.
 */
export interface KitOps {
  readonly kit: CryptoKitID;
  /**
   * Seal plaintext for `recipientPubKey` via anonymous-sender HPKE base mode.
   * Output is `eph_pub || nonce || ciphertext+tag` (kit-specific layout).
   */
  seal(plaintext: Uint8Array, recipientPubKey: Uint8Array): Promise<Uint8Array>;
  /**
   * Open a sealed buffer using the recipient's `prvKey`.  Throws on tamper
   * or wrong key.
   */
  open(sealed: Uint8Array, prvKey: Uint8Array): Promise<Uint8Array>;
  /** Generate a fresh asymmetric keypair for this kit. */
  generateKeyPair(): Promise<KeyPair>;
}

/**
 * AmpCrypto is the session-bound surface a vanilla-JS consumer reaches for:
 * `await client.seal(plaintext)` / `await client.open(sealed)` against the
 * session member's EncryptKey.
 *
 * The session member's EncryptKey is a device-local keypair the adapter
 * resolves and installs on login (generated and persisted on first use; see
 * crypto/keystore.ts).  seal/open work immediately after login without an
 * out-of-band install.  setEncryptKey remains for overriding with a key
 * sourced elsewhere, and for clearing on logout.
 */
export interface AmpCrypto {
  /** Install the session member's EncryptKey.  Clears on logout. */
  setEncryptKey(keyPair: KeyPair | null): void;
  /** Seal plaintext to the session member (anonymous-sender to self). */
  seal(plaintext: Uint8Array): Promise<Uint8Array>;
  /** Open sealed bytes with the session member's EncryptKey. */
  open(sealed: Uint8Array): Promise<Uint8Array>;
  /** Currently-installed EncryptKey pub, or null when no session. */
  getEncryptPub(): PubKeyRef | null;
}
