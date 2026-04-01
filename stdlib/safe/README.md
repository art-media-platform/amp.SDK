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

## CryptoKit — Pluggable Crypto Suites

Each `CryptoKit` is a struct with function pointers; `nil` fields indicate unsupported capabilities.
Kits self-register via `init()` and are looked up by `CryptoKitID`.

| Kit             | ID | Symmetric | Asymmetric      | Signing   |
|-----------------|----|-----------|-----------------|-----------|
| XChaCha20Poly   | 3  | XChaCha20-Poly1305 | X25519 + XChaCha20 | —   |
| ED25519         | 2  | —         | —               | Ed25519   |

To add a new suite, define a `CryptoKit` struct and call `RegisterCryptoKit()` in `init()`.

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
├── amp.safe.proto          # Protobuf definitions
├── amp.safe.pb.go          # Generated (needs protoc regeneration after proto edits)
├── api.safe.go             # Guard, TomeStore, Enclave interfaces; CryptoKit struct + registry
├── crypto.go               # XChaCha20-Poly1305 + HKDF primitives + XChaCha20Poly kit + X25519
├── enclave.go              # Enclave implementation (thread-safe KeyTome session)
├── file_guard.go           # fileGuard — passphrase-based Guard + localTomeStore
├── yubi_guard.go           # yubiGuard — YubiKey PIV placeholder
├── safe.support.go         # KeyTome/Keyring/KeyEntry utilities, PayloadPacker/Unpacker
├── safe.support.test.go    # CryptoKit test harness
├── safe_test.go            # Integration tests
├── README.md               # This file
└── ed25519/
    ├── ed25519.go          # Ed25519 signing CryptoKit
    └── ed25519_test.go     # Tests
```
