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


## Design Principles

| Principle | Meaning | Appears as |
|-----------|---------------|---------------------|
| **Total Data Ownership** | Assured data accessibility, non-deniability, and portability | Append-only signed `TxJournal` + content-addressed blob store — export, verify, and restore from any peer |
| **Total Data Privacy** | Only designated owners have cryptographic access | Epoch keys split via HKDF into `content_key` (encryption) and `proof_key` (relay verification); relay vaults *cannot* decrypt |
| **Community-Centric Permissions** | Nested hierarchies, local authority, flexible governance | Planet → channel permission model; admins rotate epoch keys, member removal triggers re-keying — no central authority |
| **Off-Grid First** | Data accessible and updatable without the internet | Transport-agnostic sync over [Reticulum](https://reticulum.network/)/[LoRa](https://en.wikipedia.org/wiki/LoRa), TCP, UDP — even sneakernet; all sync is [CRDT](https://en.wikipedia.org/wiki/Conflict-free_replicated_data_type), so offline edits merge deterministically |
| **Usability & Accessibility** | Open protocols for any technical skill level | The [amp-web-SDK](amp-web/) (`@art-media-platform/web`) TypeScript SDK with React hooks — web apps, menu cells, and spatial UIs speak one adapter |
| **Spatial UX / App Support** | Integrated real-time 3D with full workstation power |  C library `libampd` embeds into Unity / Unreal apps; UI panels render as 2D overlay, world-space canvas, or XR |
| **Hardware Agnostic** | No restriction on device count or type | Builds for iOS, Android, macOS, Windows, Linux; one binary serves TCP clients and HTTP browsers; same protocol over radio and fiber |
| **Pluggable & Extensible** | Anyone can grow, enhance, or fork | Apps register via [`amp.Registry`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.host.go); a new [`amp.AppModule`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.apps.go) = a new capability; transport is an interface, not a dependency |
| **Gatekeeperless** | No third parties to deploy, access, or manage data | `make ampd` and then you run a sovereign peer |
| **Distributed Redundancy** | Built-in replication, recovery, and failure tolerance | Vault-to-vault sync with no global clock; component failure is expected and tolerated — three nodes across two countries survive anything short of extinction |


## Planet

The core abstraction is a **planet** — a cryptographic governance enclosure maintaining membership, channels, encryption keys, and history.  A planet is not a server.  It is a cryptographic identity shared among its members, replicated across their devices and any relay nodes they choose to trust.

A planet contains **channels** — addressed as `(NodeID, AttrID)` cells with a behavior contract.  Members post **transactions** ([`TxMsg`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.apps.go)) that propagate through whatever transports are available — TCP, UDP, USB stick, or mesh-networking.  Every `TxMsg` is signed by its author and optionally encrypted with the planet's current epoch key.

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

## Vaults

A **vault** is the storage-and-sync half of a node: it holds a planet's signed transaction log and content-addressed blobs and reconciles them with other vaults holding the same planet.  Any device can run one — phone, Raspberry Pi, always-on server — so a planet lives wherever its members and the relays they trust keep a copy.

**Offline-first, convergence-second.**  A vault accepts writes with no network at all; they land in the local log and propagate when a peer is next reachable.  Two vaults reconcile by exchanging a [Merkle](https://en.wikipedia.org/wiki/Merkle_tree) range hash over their transaction sets and streaming only the difference — `O(log n)` to find a gap, then a content-addressed transfer to close it.  The hash binds entry *bytes*, not just their IDs, so a corrupted or substituted entry surfaces as a mismatch and re-heals instead of spreading.  CRDT addressing makes the merged result identical on every replica, whatever the arrival order.

**Zero-knowledge by construction.**  A vault checks that a message came from a legitimate planet member — via an [HMAC](https://en.wikipedia.org/wiki/HMAC) membership proof keyed to the epoch — without decrypting it or learning who authored it.  A seized vault yields no private data: only encrypted content, membership proofs, and routing metadata (PlanetID, EpochID, activity timeline).  An operator, willing or coerced, can hand over that much and never the data inside or its author — the precise sense in which a vault is safe to rent in an adversarial jurisdiction, with one honest limit: the metadata alone can still support traffic analysis.

**Self-certifying peers.**  Vaults authenticate by key, not by certificate authority: a peer's identity *is* a hash of its signing key, proven by a mutual challenge-response handshake — no CA to trust or breach.  Resource budgets bind to that proven identity, so one peer cannot masquerade as many and the metering holds against a sybil flood without a central allowlist.

**Bootstrap, made concrete.**  Reaching a *first* peer is the step most peer-to-peer designs hand-wave; AMP doesn't.  Every invite carries one or more `VaultAddrs`, so a new member joins by dialing the endpoints in their invite — no peer-scanning, no public DHT to seed.  Past first contact AMP is transport-agnostic (TCP, a Reticulum mesh, a LAN, a rendezvous you run) and leans on the transport for discovery rather than reinventing NAT traversal; reach any peer that holds a planet and you converge to its full state, verifying every byte yourself.  The limit worth naming is availability, not trust: a planet whose only holders are offline waits until one returns, so a real deployment keeps a few always-on vaults for the planets it depends on — which zero-knowledge makes safe to rent.

## Backup, Restore, Fork

A planet has two portable forms with sharply different jobs:

- **Chronicle** — every signed TxMsg, verbatim.  Source authority preserved.  Used for backup, offline SSD transport (hand-carry 10TB across a denied environment), and history compaction.  A new vault can replay a Chronicle and re-verify every signature with zero trust in the carrier.
- **Codex** — resolved CRDT state, history discarded.  Authority resets on import.  Used for a **Fork** (a new planet inherits state from a parent, records a [`PlanetOrigin`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/amp.core.proto) pointer, and proceeds under its own governance) and for importing data from non-AMP sources.

Forks are morally neutral — the same primitive serves a community pruning bad-faith members and a dissident escaping a captured custodian.  The protocol does not encode legitimacy.

## Identity

Members are identified by a `MemberID` derived from a canonic identity URI (`tag.NameFrom("eth:0xabc…").ID` and similar).  The substrate is identity-method-agnostic: verification reduces to `kit.Signing.Verify` against whatever `safe.Kit` the URI resolves to.  Shipped login flows: EVM wallet (EIP-4361/SIWE), email/password, and W3C [DID](https://www.w3.org/TR/did-1.0/) (login-only — `did:key` Ed25519 and `did:pkh:eip155`, the latter folding onto the same member as the wallet path).  Additional DID curves/methods (`did:key` P-256/secp256k1, `did:pkh:solana`, `did:web`) and hardware-token (YubiKey) login are on the v300 trajectory — the kit registry already covers their crypto; what remains per method is the URI-decode/verify surface.


## Federation & Naming

Planets find each other through **federation** — no central registry, no DNS authority.  A planet publishes a signed [`Brand`](amp/amp.core.proto) record naming itself, and a **NameService** resolves a human FQDN to the planet `UID` that answers for it.  Resolution rides the same zero-knowledge mesh: a peer answers from records it has synced off the verified planet-public governance stream, so a host resolves over exactly the federations it has joined.

- **Brand** — a planet's self-describing identity: display name, deep-link schemes, install targets, and operational fields (CORS origins, share-planet binding).  Signed and chronicle-tracked, so rotating any of it is a signed write, not an edit-and-restart.
- **NameService** — `Resolve(FQDN) → planet`, ranked `Search`, and federation-peer enumeration for cross-federation forwarding.  `ampd` exposes it natively and over the web wire (`/api/v1/resolve`, `/api/v1/search`, `/api/v1/federation/peers`).
- **Deep links** — an inbound web Host resolves through NameService to its planet's `Brand`, which renders the platform-detecting landing page — the same substrate-native path whether the link is yours or a federated peer's.

**What naming does *not* claim.**  There is no global name authority — by design, which also means there is no universal namespace.  An FQDN resolves only within the federations a host has joined; two federations may mint the same name and neither is canonical.  The durable identity is the planet `UID` — self-certifying and collision-free — with the human-readable name a convenience layered over it; first contact with a federation you have not joined still needs an out-of-band pointer (a UID, an invite, or a peer that already carries the record).  That is the same cost inverted into the benefit: no registry to capture, censor, or bill, and you resolve over exactly the mesh you chose to trust.


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

At the v300 wire freeze, a planet's root will carry `PlanetMod` (declared dependencies) and `PlanetLock` (hash-pinned content addresses) — together a live, hardware-signed [SBOM](https://www.cisa.gov/sbom).  Publisher signatures root at hardware tokens (YubiKey P-256 / WebAuthn); dependency resolution uses Go's Minimum Version Selection algorithm.  Strictly more powerful than SPDX or CycloneDX file formats: AMP's SBOM is the runtime substrate, content-addressed end-to-end, replayable to any historical state.

Because it is a structural property of every planet rather than a bolt-on, federal contractors, regulated verticals, and supply-chain-conscious vendors inherit it by construction.


## Architecture

```
amp.Host
  ├── app.www.WebService [::]:5193  # HTTP front door
  │    ├── /www/*   → asset streaming (data.Publisher)
  │    ├── /link/*  → deep links
  │    └── /*       → static sites · REST · WebSocket · menu cells
  ├── tcp.HostService [::]:5192     # TCP client transport
  ├── vault.Controller              # chronicle (signed TxMsg log) + sync
  │    ├── TxJournal                # append-only signed transaction log
  │    ├── BlobStore                # content-addressed encrypted blobs
  │    └── vault.Transport          # Reticulum · TCP · UDP · mesh
  └── amp.Session                   # one per connected client
       ├── safe.Enclave             # identity keys, never leave the device
       ├── safe.EpochKeyStore       # symmetric epoch keys, per planet epoch
       └── AppInstances
            ├── app.home            # member identity, planet subscriptions
            ├── app.members         # epoch-key extraction, governance
            ├── app.cabinets        # persistent key-value storage
            ├── app.codex           # Chronicle / Codex export & restore
            └── your.app            # custom functionality
```

Every long-lived object is a node in a [`task.Context`](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/task/api.task.go) tree.  Closing a parent closes all children.  The host operates fully offline — sync happens opportunistically when connectivity is available.

Apps register via [`amp.Registry`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.host.go); each [`amp.AppModule`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.apps.go) is a factory that creates instances on demand.  A client *pins* a node by URL, the host resolves it to an app, and the app pushes [`amp.TxMsg`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.apps.go) updates back through the session.

### Packages of Interest

| Package | Purpose |
|---------|---------|
| [`amp/`](https://github.com/art-media-platform/amp.SDK/tree/main/amp) | Core types: [`TxMsg`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.apps.go), [`Session`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.host.go), [`AppModule`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.apps.go), CRDT bindings |
| [`stdlib/safe/`](https://github.com/art-media-platform/amp.SDK/tree/main/stdlib/safe) | [`Enclave`](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/safe/api.safe.go), [`Kit`](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/safe/api.safe.go), key management, AEAD, HKDF |
| [`stdlib/tag/`](https://github.com/art-media-platform/amp.SDK/tree/main/stdlib/tag) | Universal tagging and addressing |
| [`stdlib/task/`](https://github.com/art-media-platform/amp.SDK/tree/main/stdlib/task) | Goroutine lifecycle management (parent-child process model) |
| [`amp/webapi/`](amp/webapi/) | HTTP/JSON wire contract for the web SDK — the `/api/v1/*` shapes |
| [`amp-web/`](amp-web/) | [`@art-media-platform/web`](amp-web/) — the **amp-web-SDK** TypeScript client (React hooks); contract in [`amp-web/SKILL-amp-web-SDK.md`](amp-web/SKILL-amp-web-SDK.md) |


### Integration Flow

This repo is the SDK — lightweight, dependency-minimal, safe to add to any Go project.

1. Add [amp.SDK](https://github.com/art-media-platform/amp.SDK) to your Go project
2. Implement an [`amp.AppModule`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.apps.go) for your functionality
3. Clone amp.planet and register your module
4. `make build` produces `ampd` (standalone server) and `libampd` (embeddable C library)
5. For web apps, use the [**amp-web-SDK**](amp-web/) — the [`@art-media-platform/web`](amp-web/) TypeScript SDK (React hooks); see [`amp-web/SKILL-amp-web-SDK.md`](amp-web/SKILL-amp-web-SDK.md) for the full contract

> **Deep design specs.** The PRDs behind this SDK — architecture, security/sync, crates, Manifold, commerce — are indexed in [`amp.planet/AOM/AOM.md`](https://github.com/art-media-platform/amp.planet/blob/main/AOM/AOM.md).

---

<p align="center">
  <img src="media/art-media-platform.svg" alt="art.media.platform" width="280">
</p>
