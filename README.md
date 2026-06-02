<p align="center">
  <img src="media/art-media-platform.svg" alt="art.media.platform" width="280">
</p>

**art.media.platform** ("AMP") is an open protocol and native 3D runtime for federated, end-to-end-encrypted applications.  Every participant holds their own keys.  Every device can be a full peer.  Content propagates through a mesh of independent nodes — no central server, no corporate intermediary, no single point of failure.

This repository is the **SDK**: a dependency-light Go library defining the wire format, CRDT addressing model, key/identity primitives, and `AppModule` interface.  Pair it with amp.planet to produce:

- `ampd` — standalone server / vault (macOS, Windows, Linux, Raspberry Pi, Android, Graphene)
- `libampd` — embeddable C library for Unity and Unreal clients
- [**amp-web-SDK**](amp-web/) — the [`@art-media-platform/web`](amp-web/) TypeScript SDK (React hooks) for web apps
- Reticulum gateway for transport over LoRa / packet radio / I2P

Born from [PLAN](https://plan-systems.org/plan-technology-components/) (2017) and its cryptographic [proof of correctness](https://github.com/plan-systems/design-docs/blob/master/PLAN-Proof-of-Correctness.md), AMP is the third generation of this architecture.


## The Problem

Your data lives on someone else's servers, encrypted with someone else's keys, subject to someone else's terms of service.  When that company gets acquired, shuts down, or decides to change the rules — you lose.

In crisis scenarios — natural disasters, infrastructure collapse, conflict zones — centralized platforms fail precisely when communication matters most.  Cell towers go down, internet links sever, and the tools people depend on become unreachable.


## Planet

The core abstraction is a **planet** — a cryptographic governance enclosure maintaining membership, channels, encryption keys, and history.  A planet is not a server.  It is a cryptographic identity shared among its members, replicated across their devices and any relay nodes they choose to trust.

A planet contains **channels** — addressed as `(NodeID, AttrID)` cells with a behavior contract.  Members post **transactions** ([`TxMsg`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go)) that propagate through whatever transports are available — TCP, UDP, USB stick, or mesh-networking.  Every `TxMsg` is signed by its author and optionally encrypted with the planet's current epoch key.

| Mode | Signed | Encrypted | Who Can Read (Decrypt) | Who Can Write |
|------|--------|-----------|--------------|---------------|
| **Public** | Yes | No | Anyone | Planet members |
| **Planet-Private** | Yes | Yes | Planet members | Planet members |
| **Channel-Private** | Yes | Yes | Channel members | Channel members |

A private planet can contain public channels alongside private channels.  Private channels can only be read by channel participants — even other planet members cannot decrypt them.

### Member Keypairs

Identity and key-receipt have opposing rotation needs, so AMP splits them:

- **SigningKey** — author identity.  Often hardware-bound (YubiKey P-256, MetaMask secp256k1).  Stable, rarely rotated, signs every TxMsg.
- **EncryptKey** — software-resident, cheap to rotate.  Receives sealed-box wraps of new epoch keys.  Rotated whenever the planet rotates.


### [CRDT](https://en.wikipedia.org/wiki/Conflict-free_replicated_data_type) Addressing

Every piece of state has a unique [`amp.Address`](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/tag/api.tag.go): _planet → channel → attribute → item → edit_.  When two members edit the same item offline and later sync, their edits merge automatically.  No authoritative server; every peer holds a replica; convergence is [guaranteed](https://crdt.tech/).  The [`amp.Tag` system](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/tag/README.md) provides UID derivation from strings.

### Epoch Rotation

A planet's encryption changes over time through **epoch rotation**.  When an admin rotates the epoch — to revoke a member, respond to a compromise, or as routine hygiene — a new symmetric key is generated and sealed-box-wrapped to each active member's [EncryptKey](#member-keypairs).  Revoked members never receive the new key.  Historical content remains readable with old epoch keys; new content is sealed under the new epoch.  Private-channel keys derive from `HKDF(channel_epoch || planet_epoch)`, so rotating the planet implicitly invalidates all channel keys without per-channel rotation.

### Zero-Knowledge Relay

Relay nodes ("vaults") store and forward encrypted TxMsgs.  A vault verifies that a message was authored by a legitimate planet member — via an [HMAC](https://en.wikipedia.org/wiki/HMAC) membership proof derived from the epoch key — without ever decrypting the content or learning which member authored it.

**A seized vault yields zero content.**  It holds opaque encrypted blobs and membership proofs and exposes nothing else.  Routing metadata (PlanetID, EpochID, activity timeline) are observable; payloads and author identity do not.  You can run vault infrastructure in jurisdictions with adversarial data-access laws and sleep soundly.

## Backup, Restore, Fork

A planet has two portable forms with sharply different jobs:

- **Chronicle** — every signed TxMsg, verbatim.  Source authority preserved.  Used for backup, offline SSD transport (hand-carry 10TB across a denied environment), and history compaction.  A new vault can replay a Chronicle and re-verify every signature with zero trust in the carrier.
- **Codex** — resolved CRDT state, history discarded.  Authority resets on import.  Used for a **Fork** (a new planet inherits state from a parent, records a [`PlanetOrigin`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/amp.core.proto) pointer, and proceeds under its own governance) and for importing data from non-AMP sources.

Forks are morally neutral — the same primitive serves a community pruning bad-faith members and a dissident escaping a captured custodian.  The protocol does not encode legitimacy.

## Identity

Members are identified by a `MemberID` derived from a canonic identity URI (`tag.NameFrom("eth:0xabc…").ID` and similar).  The substrate is identity-method-agnostic: verification reduces to `kit.Signing.Verify` against whatever `KitSpec` the URI resolves to.  Shipped login flows: EVM wallet (EIP-4361/SIWE), email/password, and W3C [DID](https://www.w3.org/TR/did-1.0/) (login-only — `did:key` Ed25519 and `did:pkh:eip155`, the latter folding onto the same member as the wallet path).  Additional DID curves/methods (`did:key` P-256/secp256k1, `did:pkh:solana`, `did:web`) and hardware-token (YubiKey) login are on the v300 trajectory — the kit registry already covers their crypto; what remains per method is the URI-decode/verify surface.


## Federation & Naming

Planets find each other through **federation** — no central registry, no DNS authority.  A planet publishes a signed [`Brand`](amp/amp.core.proto) record naming itself, and a **NameService** resolves a human FQDN to the planet `UID` that answers for it.  Resolution rides the same zero-knowledge mesh: a peer answers from records it has synced off the verified planet-public governance stream, so a host resolves over exactly the federations it has joined.

- **Brand** — a planet's self-describing identity: display name, deep-link schemes, install targets, and operational fields (CORS origins, share-planet binding).  Signed and chronicle-tracked, so rotating any of it is a signed write, not an edit-and-restart.
- **NameService** — `Resolve(FQDN) → planet`, ranked `Search`, and federation-peer enumeration for cross-federation forwarding.  `ampd` exposes it natively and over the web wire (`/api/v1/resolve`, `/api/v1/search`, `/api/v1/federation/peers`).
- **Deep links** — an inbound web Host resolves through NameService to its planet's `Brand`, which renders the platform-detecting landing page — the same substrate-native path whether the link is yours or a federated peer's.


## AI

AMP's channel attribute addressing is a natural fit for AI agents.  An AI daemon is given access to explicit planets or channels — and *only* those.  This provides structural compartmentalization: an AI assistant processing `chat-support` never receives keys for `medical-records` or `financial-ledger`, not because of a policy document, but because it has not been *given* those keys.

- **Scoped by default** — implicit cryptographic enforcement
- **Auditable** — every TxMsg is signed, attributed, and immutable
- **Revocable** — rotate the channel epoch key and the AI loses access, no token dance


## Reticulum

[Reticulum](https://reticulum.network/) is a cryptography-based mesh networking stack and [community](https://github.com/markqvist/Reticulum) building wide-area networks on unreliable, mixed-medium infrastructure — [LoRa](https://en.wikipedia.org/wiki/LoRa) radio, packet radio, serial links, TCP, UDP, I2P.

Reticulum and AMP are architecturally aligned:
  - both are peer-to-peer by construction,
  - both separate identity from location via cryptographic keys,
  - both handle intermittent connectivity gracefully.
  - both are complementary: AMP encrypts at the application layer while Reticulum encrypts at the transport layer.

`vault.Transport` makes AMP fully transport-agnostic.  Reticulum over LoRa gives AMP nodes mesh federation without internet, cell towers, or any centralized infrastructure — disaster response teams, rural classrooms, field teams in denied environments, all running encrypted CRDT channels over radio, offering [3D spatial collaboration](https://plan-systems.org/2025/04/17/25-use-cases-for-a-spatial-os-created-with-plan-3d/).


## Why a 3D Runtime?

The future of computing is spatial.  AR/VR headsets, digital twins, geospatial logistics, immersive collaboration — these are active markets with real deployments today.  But spatial platforms have been a walled garden: Meta's Horizon, Apple's visionOS, Google's ARCore.

AMP provides the security and communication layer that spatial applications need without corporate lock-in.  It ships as an embeddable C library for [Unity](https://unity.com) and [Unreal](https://unrealengine.com), or as a standalone server binary (`ampd`) for headless operation.

A Unity app with AMP embedded has end-to-end encrypted federated communication, offline-capable CRDT state, authenticated media streaming, and mesh networking — all without a single line of server code or a single dependency on a cloud provider.

**Platforms:** Windows, macOS, Linux, iOS, Android, XR headsets (Meta Quest, Apple Vision).  The same binary that runs a headless vault on a Raspberry Pi serves a VR collaboration space.


## Package Management

A planet's root carries `PlanetMod` (declared dependencies) and `PlanetLock` (hash-pinned content addresses) — together a live, hardware-signed [SBOM](https://www.cisa.gov/sbom).  Publisher signatures root at hardware tokens (YubiKey P-256 / WebAuthn); dependency resolution uses Go's Minimum Version Selection algorithm.  Strictly more powerful than SPDX or CycloneDX file formats: AMP's SBOM is the runtime substrate, content-addressed end-to-end, replayable to any historical state.

Federal contractors, regulated verticals, and supply-chain-conscious vendors do not bolt this on — they inherit it from day one.


## Architecture

```
amp.Host
  ├── vault.Controller         # chronicle (signed TxMsg log) + sync engine
  │     └── vault.Transport    # Reticulum, TCP, UDP, ...
  ├── vault.BlobStore          # content-addressed encrypted blobs
  └── amp.Session              # one per connected client
       ├── safe.Enclave        # identity keys, never leaves device
       ├── safe.EpochKeyStore  # symmetric epoch keys, per planet epoch
       └── AppInstances
            ├── app.home       # member identity, planet subscriptions
            ├── app.members    # epoch key extraction, governance
            ├── app.cabinets   # persistent key-value storage
            ├── app.www        # REST / WebSocket / asset streaming
            ├── app.codex      # Chronicle / Codex export + restore
            └── your.app       # custom functionality
```

Every long-lived object is a node in a [`task.Context`](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/task/api.task.go) tree.  Closing a parent closes all children.  The host operates fully offline — sync happens opportunistically when connectivity is available.

### Packages of Interest

| Package | Purpose |
|---------|---------|
| [`amp/`](https://github.com/art-media-platform/amp.SDK/tree/main/amp) | Core types: [`TxMsg`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go), [`Session`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.host.go), [`AppModule`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go), CRDT bindings |
| [`stdlib/safe/`](https://github.com/art-media-platform/amp.SDK/tree/main/stdlib/safe) | [`Enclave`](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/safe/api.safe.go), [`KitSpec`](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/safe/api.safe.go), key management, AEAD, HKDF |
| [`stdlib/tag/`](https://github.com/art-media-platform/amp.SDK/tree/main/stdlib/tag) | Universal tagging and addressing |
| [`stdlib/task/`](https://github.com/art-media-platform/amp.SDK/tree/main/stdlib/task) | Goroutine lifecycle management (parent-child process model) |
| [`amp/webapi/`](amp/webapi/) | HTTP/JSON wire contract for the web SDK — the `/api/v1/*` shapes |
| [`amp-web/`](amp-web/) | [`@art-media-platform/web`](amp-web/) — the **amp-web-SDK** TypeScript client (React hooks); contract in [`amp-web/SKILL-amp-web-SDK.md`](amp-web/SKILL-amp-web-SDK.md) |


### Integration Flow

This repo is the SDK — lightweight, dependency-minimal, safe to add to any Go project.

1. Add [amp.SDK](https://github.com/art-media-platform/amp.SDK) to your Go project
2. Implement an [`amp.AppModule`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go) for your functionality
3. Clone amp.planet and register your module
4. `make build` produces `ampd` (standalone server) and `libampd` (embeddable C library)
5. For web apps, use the [**amp-web-SDK**](amp-web/) — the [`@art-media-platform/web`](amp-web/) TypeScript SDK (React hooks); see [`amp-web/SKILL-amp-web-SDK.md`](amp-web/SKILL-amp-web-SDK.md) for the full contract

> **Deep design specs.** The PRDs behind this SDK — architecture, security/sync, crates, Manifold, commerce — are indexed in [`amp.planet/docs/INDEX.md`](https://github.com/art-media-platform/amp.planet/blob/main/docs/INDEX.md).

---

<p align="center">
  <img src="media/art-media-platform.svg" alt="art.media.platform" width="280">
</p>
