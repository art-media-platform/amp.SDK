# amp.SDK/stdlib/safe

Secure key storage and retrieval for the **amp** platform.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                         Enclave                         │
│   (runtime session — live KeyTome, fully internal)      │
│                                                         │
│   ImportKey()    — merge a keypair into a keyring       │
│   GenerateKey()  — generate + register a keypair        │
│   FetchPubKey()  — resolve a KeyRef to its PubKey       │
│   CanSign()      — is the PRIVATE half actually held?   │
│   SignRaw()      — sign a registry-derived digest       │
│   EncryptSym() / DecryptSym()   — symmetric seal/open   │
│   OpenFromPub()  — open a sealed box for a held key     │
│   ExportSymmetricKey()  — symmetric material only       │
│   Close()        — re-seal, persist, zero               │
│                                                         │
│   Open:   TomeStore.Load() → Guard.UnwrapDEK()          │
│           → AEAD decrypt → KeyTome                      │
│   Close:  new DEK → Guard.WrapDEK()                     │
│           → AEAD encrypt(KeyTome) → TomeStore.Save()    │
└─────────────────────────────────────────────────────────┘
          │                             │
    ┌─────▼──────┐              ┌───────▼───────┐
    │   Guard    │              │   TomeStore   │
    │ (DEK wrap) │              │ (sealed tome) │
    └────┬───────┘              └───────────────┘
         │
    ┌────▼────┐
    │fileGuard│
    │ (HKDF+  │
    │   pass) │
    └─────────┘
```

`fileGuard` is the Guard this package ships (`api.safe.go` names it as the sole
implementation).  A hardware Guard is a host-side concern — see
[Hardware Targets](#hardware-targets).

### Two Key Stores

`Enclave` holds **identity** keys — signing and asymmetric keypairs, one keyring
per identity.  Symmetric **epoch** keys live in a separate interface,
[`EpochKeyStore`](api.safe.go), because their access pattern is different:
millions of keys per member across planets and channels, keyed by
`(containerID, epochID, role)`, exported for subkey derivation, and hot/cold
separated so only current epochs stay resident.

| | `Enclave` | `EpochKeyStore` |
|---|---|---|
| Holds | identity keypairs | symmetric epoch keys |
| Keyed by | keyring UID + `KeyRef` | `(containerID, epochID, KeyRole)` |
| Private material leaves? | never (except `ExportSymmetricKey`) | yes — `GetKey` returns bytes to derive subkeys |
| Rotation unit | per key | per epoch (`PutKey` / `SetCurrentEpoch` / `ShredKeys`) |

Both seal to disk through the same Guard/DEK mechanism, and both return
`ErrStoreClosed` once `Close` has sealed the session — "no custody" reads as
retryable, never as forgery.

## Pluggable Crypto

A `safe.Kit` bundles two independent capability axes — `Signing` and `Encrypt` (asymmetric ECDH) — either of which may be `nil` when a kit does not offer it.  Symmetric AEAD is kit-agnostic and lives on the `safe` package directly (`SealAEAD` / `OpenAEAD`).  Kits self-register via `init()` and are looked up by `CryptoKitID`.

`CryptoKitID` is **`= tag.UID`** (an alias, `api.safe.go`) — a 128-bit UID derived
from the kit's name, not a small enum.  The namespace is open: a sovereign may
roll its own suite and name it without asking anyone.  Open naming is not open
trust — `safe.CryptoKit(id)` fails closed for any unregistered kit, and the
trusted set is pinned in signed `EpochTerms`.  The stock UIDs are baked at forge
time from [`safe.consts.sdl`](safe.consts.sdl); use `safe.Crypto.{Kit}.ID`.

| Kit       | Name (→ `CryptoKitID`)  | Asymmetric (Encrypt) | Signing                                | Status               |
|-----------|-------------------------|----------------------|----------------------------------------|----------------------|
| Poly25519 | `amp.crypto.poly25519`  | X25519 ECDH          | Ed25519                                | registered (default) |
| P256      | `amp.crypto.p256`       | ECDH P-256           | ECDSA P-256 + SHA-256 (NIST; YubiKey PIV)   | registered           |
| Secp256k1 | `amp.crypto.secp256k1`  | ECDH secp256k1       | ECDSA secp256k1 + Keccak-256 (crypto-wallet) | registered (in amp.planet `app.evmwallet`) |

Symmetric AEAD for every kit is XChaCha20-Poly1305.  To add a suite, define a `Kit` (set `Signing` and/or `Encrypt`) and call `RegisterCryptoKit(kit)` from `init()`.  The hash sibling is `RegisterHashKit(spec)`.

## Cryptographic Choices

| Component      | Algorithm                 | Rationale |
|----------------|---------------------------|-----------|
| AEAD cipher    | XChaCha20-Poly1305        | 256-bit key, 192-bit nonce (no nonce reuse concern), constant-time, no AES-NI dependency |
| KDF            | HKDF-SHA256               | Standard, deterministic, suitable for key stretching from high-entropy root material |
| DEK wrap       | HKDF(root, salt) → AEAD   | Fresh salt per wrap = fresh wrapping key, forward secrecy on each seal |
| Asymmetric     | X25519 ECDH + HKDF → AEAD | Modern elliptic curve Diffie-Hellman, 32-byte keys |
| YubiKey wrap   | ECDH P-256 + HKDF → AEAD  | On-card private key never leaves hardware |
| Signing        | Ed25519                   | Standard library `crypto/ed25519`, compact signatures |
| Hashing        | BLAKE2s-256 (default)     | Fast, 256-bit, used for payload signing.  Not fixed: `HashKitID` is carried per-epoch in `EpochTerms`, so an epoch may elect BLAKE3-256, SHA2-256, or SHA3-256 instead |


## Hardware Targets

### 1. File Guard (mobile/desktop)
- Root material: passphrase from OS keychain or biometric unlock
- Storage: local filesystem (SSD)
- Use case: phones, laptops, dev machines

### 2. YubiKey Guard (hardware dongle) — host-side, not in this package
- Root material: PIV key on YubiKey (P-256 ECDH)
- Use case: USB-C security dongle, merch/keychains
- **Ships in amp.planet**, not in this SDK: `amp/apps/app.yubikey`, with
  `github.com/go-piv/piv-go/v2` in *amp.planet's* `go.mod`.  This package's
  `go.mod` requires only `pkg/errors`, `golang.org/x/crypto`, and `protobuf`;
  `fileGuard` is the only Guard it implements.

### 3. Future: NFC Wearable
- Root material: NFC tap triggers phone-side decryption
- Use case: NFC ring/wristband for teens + young adults
- The phone holds the encrypted tome; the ring authorizes


## Files

```
safe/
├── safe.proto              # Protobuf definitions (KeyType, HashKitID, KeyRole, SealedTome, ...)
├── safe.pb.go              # Generated from safe.proto (regen via `make generate`)
├── safe.consts.sdl         # CryptoKit identity UIDs + the canonical phrase wordlist
├── safe.consts.go          # Generated from safe.consts.sdl by `forge const`
├── api.safe.go             # Guard, TomeStore, Enclave, EpochKeyStore interfaces; Kit + KeySpec
├── registry.go             # RegisterCryptoKit / RegisterHashKit — init()-only, fail-closed lookup
├── crypto.go               # XChaCha20-Poly1305 AEAD + HKDF primitives + X25519
├── enclave.go              # Enclave implementation (thread-safe KeyTome session)
├── epoch_keys.go           # EpochKeyStore — symmetric epoch keys, per (container, epoch, role)
├── file_guard.go           # fileGuard — passphrase-based Guard + localTomeStore
├── phrase.go               # mnemonic phrase ↔ key material
├── safe.keys.go            # KeyRef / PubKey / SymKey / KeyPair value types
├── safe.sign.go            # SigningDigest / SignDomain / VerifyDomain — domain-separated signing
├── safe.support.go         # KeyTome/Keyring/KeyEntry utilities, PayloadPacker/Unpacker
├── README.md               # This file
├── poly25519/              # Poly25519 Kit (X25519 + Ed25519)
└── p256/                   # P256 Kit (ECDH P-256 + ECDSA P-256)
```
