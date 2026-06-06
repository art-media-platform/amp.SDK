# amp.SDK/stdlib/safe

Secure key storage and retrieval for the **amp** platform.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                       Enclave                           │
│   (runtime session — live KeyTome, fully internal)      │
│                                                         │
│   ImportKeys()    — merge external keys into the tome   │
│   GenerateKeys()  — generate + merge new key entries    │
│   FetchKeyInfo()  — look up a key's public info         │
│   DoCryptOp()     — sign, sym-encrypt, asym-encrypt     │
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
    ┌────┴────────────┐
    │                 │
┌───▼─────┐     ┌─────▼──────┐
│fileGuard│     │ yubiGuard  │
│ (HKDF+  │     │(PIV ECDH   │
│   pass) │     │ via go-piv)│
└─────────┘     └────────────┘
```

## Pluggable Crypto

A `safe.Kit` bundles two independent capability axes — `Signing` and `Encrypt` (asymmetric ECDH) — either of which may be `nil` when a kit does not offer it.  Symmetric AEAD is kit-agnostic and lives on the `safe` package directly (`SealAEAD` / `OpenAEAD`).  Kits self-register via `init()` and are looked up by `CryptoKitID` (`safe.proto`).

| Kit       | ID | Asymmetric (Encrypt) | Signing                                | Status               |
|-----------|----|----------------------|----------------------------------------|----------------------|
| Poly25519 | 1  | X25519 ECDH          | Ed25519                                | registered (default) |
| P256      | 2  | ECDH P-256           | ECDSA P-256 + SHA-256 (NIST; YubiKey PIV)   | registered           |
| Secp256k1 | 3  | ECDH secp256k1       | ECDSA secp256k1 + Keccak-256 (crypto-wallet) | registered (in amp.planet `app.evmwallet`) |

Symmetric AEAD for every kit is XChaCha20-Poly1305.  To add a suite, define a `Kit` (set `Signing` and/or `Encrypt`) and call `RegisterKit()` in `init()`.

## Cryptographic Choices

| Component      | Algorithm                 | Rationale |
|----------------|---------------------------|-----------|
| AEAD cipher    | XChaCha20-Poly1305        | 256-bit key, 192-bit nonce (no nonce reuse concern), constant-time, no AES-NI dependency |
| KDF            | HKDF-SHA256               | Standard, deterministic, suitable for key stretching from high-entropy root material |
| DEK wrap       | HKDF(root, salt) → AEAD   | Fresh salt per wrap = fresh wrapping key, forward secrecy on each seal |
| Asymmetric     | X25519 ECDH + HKDF → AEAD | Modern elliptic curve Diffie-Hellman, 32-byte keys |
| YubiKey wrap   | ECDH P-256 + HKDF → AEAD  | On-card private key never leaves hardware |
| Signing        | Ed25519                   | Standard library `crypto/ed25519`, compact signatures |
| Hashing        | BLAKE2s-256 (default)     | Fast, 256-bit, used for payload signing |


## Hardware Targets

### 1. File Guard (mobile/desktop)
- Root material: passphrase from OS keychain or biometric unlock
- Storage: local filesystem (SSD)
- Use case: phones, laptops, dev machines

### 2. YubiKey Guard (hardware dongle)
- Root material: PIV key on YubiKey (P-256 ECDH)
- Requires: `github.com/go-piv/piv-go/v2`
- Use case: USB-C security dongle, merch/keychains

### 3. Future: NFC Wearable
- Root material: NFC tap triggers phone-side decryption
- Use case: NFC ring/wristband for teens + young adults
- The phone holds the encrypted tome; the ring authorizes


## Files

```
safe/
├── safe.proto              # Protobuf definitions (KeyType, CryptoKitID, SealedTome, ...)
├── safe.pb.go              # Generated from safe.proto (regen via `make generate`)
├── api.safe.go             # Guard, TomeStore, Enclave, EpochKeyStore interfaces; Kit + registry
├── crypto.go               # XChaCha20-Poly1305 AEAD + HKDF primitives + X25519
├── enclave.go              # Enclave implementation (thread-safe KeyTome session)
├── epoch_keys.go           # EpochKeyStore — symmetric epoch keys, per (container, epoch, role)
├── file_guard.go           # fileGuard — passphrase-based Guard + localTomeStore
├── yubi_guard.go           # yubiGuard — YubiKey PIV Guard
├── phrase.go               # mnemonic phrase ↔ key material
├── safe.keys.go            # KeyRef / PubKey / SymKey / KeyPair value types
├── safe.support.go         # KeyTome/Keyring/KeyEntry utilities, PayloadPacker/Unpacker
├── README.md               # This file
├── poly25519/              # Poly25519 Kit (X25519 + Ed25519)
└── p256/                   # P256 Kit (ECDH P-256 + ECDSA P-256)
```
