# SKILL: amp-web-SDK

> **What this document is:** Instructions for an AI coding agent (Claude Code, Cursor, Replit Agent, etc.) — and a contract for human web developers — to build web applications that persist data through the art.media.platform (amp) vault infrastructure. Drop this file into any web project. Generated code uses the `@art-media-platform/web` library to talk to `app.www`, the unified HTTP service inside `ampd`.

> **What amp is (30-second version):** amp is a decentralized storage and communication protocol where data is encrypted, signed, and replicated across independent nodes ("vaults"). Vaults relay traffic without reading content. Every host is a full peer — your app works offline; sync is additive. Your web app is a disposable UI layer; amp is the durable substrate underneath.

---

## 0. Read This First (AI agents especially)

**You are building a web client. amp is already running as a service. You will
not build, run, compile, embed, or `go build` any server.** If you catch yourself
cloning amp.planet / amp.SDK, writing Go, running `make`, or "standing up a node,"
stop — wrong path. Your whole footprint is one TypeScript package + HTTPS calls.

- **Your dependency** is the SDK bundle: `npm install ./amp-web-SDK` (a `file:`
  dependency). The bare-name `npm install @art-media-platform/web` from the public
  registry is **not** available during beta — it lands when the SDK publishes at
  **v400**. We're pre-v400 today, so install the versioned bundle locally.
- **Your server already exists.** You connect over HTTPS to an `ampd` node an
  **operator runs for you** — you do not run, build, or host one. For Maplable that
  node is **`https://prod.plan.tools`**. (`:5193`, seen elsewhere in this doc, is
  only the local-dev default; an operated node is plain TLS on 443.)
- **First goal — one round-trip:** log in → write one item → read it back (~20
  lines, below). If that round-trips, you're talking to the vault and the rest of
  this document is detail.
- **Hello-world needs no invite; *joining a federation* does.** Wallet login
  already binds you to a private planet you can write to immediately (the
  round-trip below). But registering your planets / names into a federation — and
  being discoverable across orgs — **requires membership in it**, which is what
  accepting your invite grants (§4.7). Prove the pipe with a Tx first, then accept
  the invite to join Federation Alpha.

### Zero to first Tx

```ts
import { AmpWebClient } from '@art-media-platform/web';

const client = new AmpWebClient({
  vaultUrl:  'https://prod.plan.tools',   // the operated node (NOT one you run)
  planetTag: '<handed to you>',
});

// Log in. Wallet (SIWE) auto-provisions a private home planet — zero operator setup.
// connectWallet / signWithWallet are YOUR wallet glue (EIP-1193), not SDK calls —
// the full EIP-6963 + personal_sign version is in §5.1; the inline comments show it.
const addr      = await connectWallet();                   // (await window.ethereum.request({ method: 'eth_requestAccounts' }))[0]
const challenge = await client.getWalletChallenge(addr);
const signature = await signWithWallet(challenge.Message); // window.ethereum.request({ method: 'personal_sign', params: [challenge.Message, addr] })
const member    = await client.login({ Scheme: 'wallet', Address: addr, Signature: signature, Nonce: challenge.Nonce });

// Write your first transaction, then read it straight back.
const itemID   = await client.create('hello', 'world', { msg: 'hi' });
const { data } = await client.query('hello', 'world', { itemID });
console.log(data[0].msg);     // 'hi' — round-tripped through a real journal.  query flattens
                              // the item's Value to the top level, beside _ItemID/_FromID/… meta.
```

No Go, no node, no genesis ceremony — just a client and HTTPS.

| Do | Don't |
|---|---|
| `npm install ./amp-web-SDK` (the bundle) | `git clone` / `go build` amp.planet or amp.SDK |
| construct `AmpWebClient` at the operated `vaultUrl` | run / compile / embed `ampd`, or stand up a node |
| `login` → `create` / `query` / `subscribe` / `upload` | reimplement tag canonization (use `resolveTag`, §5.8) |
| build your React UI + cards (§8) | raw-`fetch` `/api/v1/*` (always go through the SDK) |

---

## 1. Scope of This Contract

This SKILL is the contract a web app codes against. It has one consumer and one server:

- **Server:** `app.www`, an amp app inside `ampd`, owning HTTP port 5193 (the local-dev default; an operated node terminates TLS on 443 — §0). The wire shape (REST verbs, WebSocket event format, auth model) is the contract.
- **Reference client:** `@art-media-platform/web`, a TypeScript package that wraps the wire shape with React hooks. Other languages reach the wire directly.

The SKILL describes:
1. The wire shape — what `app.www` accepts and returns.
2. The `@art-media-platform/web` API — the React-level abstraction over the wire.
3. The data-model conventions — channel/attr layout, item IDs, blobs, sealed secrets.
4. The WebRect bridge — how a single HTML **card** can run inside a Unity 3D Pane or a desktop browser without changing.

A **flagship consumer** anchors the contract: **Maplable**, an offline-first 3D mapping toolset whose `ProjectSyncService` adapter swaps from local PHP/MySQL to amp as a one-line constructor change. Maplable's channel/attr layout (`projects/snapshot`, `users/profile`, etc.) is the worked example throughout. Maplable is a **stand-in for any web client** consuming this SDK — nothing here is Maplable-specific.

**Reference docs.**

| Item | What It Is | When You Need It |
|---|---|---|
| **This SKILL** | AI-agent instructions, API reference, data model conventions, WebRect bridge IDL | Always |
| **`@art-media-platform/web`** (the client package in this bundle) | TypeScript reference client — React hooks, sealed-box helpers, cross-planet address helper | At `npm install` time |
| **`webapi/webapi.types.go`** (canonical source: `amp.SDK/amp/webapi`) | The wire contract verbatim — Go structs + JSON tags for every `/api/v1/*` shape. Mirror these field names one-to-one in any language. | Implementing a non-TS client |
| **[`SECURITY-amp-web-SDK.md`](SECURITY-amp-web-SDK.md)** | The security model a consumer relies on — encryption, identity, what a vault can and cannot see | Auditing security claims |

---

## 2. How to Use This SKILL

**AI Agent rules.** You are writing a **web client** against an `ampd` node an operator already runs — never building, running, or embedding a server (§0). When generating or modifying code for a web project that uses amp:

1. **All data operations** go through `@art-media-platform/web` (never raw `fetch` to `/api/v1/*`).
2. **File uploads** go through `useAmpUpload()`.
3. **Real-time subscriptions** are built into `useAmpQuery()`.
4. **Auth** uses `useAmpAuth()`.
5. **User-supplied secrets** (BYOK API keys, auth tokens) are sealed client-side via `client.seal()` BEFORE any `upsert` — never stored in plaintext channel items.
6. **Cards** (HTML running inside a Unity WebRect Pane or a 2D drawer) speak to amp via `window.amp` — see §8. A card is the detail view of a single item, opened on activation; lists are rendered natively by the host, not as 300 cards in 300 WebViews.
7. **Public-readable content** lives on a separate planet operating in public mode — see §6.
8. **Hard delete vs withdraw** are distinct verbs — see §7.

---

## 3. Setup

### Install

**Beta (pre-v400)** — distributed in the `amp-web-SDK` bundle, not yet on npm. Build it (`make amp-web-SDK`) or get it from your amp contact, then install the local package:

```bash
npm install ./amp-web-SDK
```

```json
{ "dependencies": { "@art-media-platform/web": "file:./amp-web-SDK" } }
```

Publishing to npm as `@art-media-platform/web` lands at v400 — until then, install the versioned bundle locally (above).

### Provider Configuration

`vaultUrl` is the HTTPS address of an `ampd` node an **operator runs for you** — you never run, build, or host one (§0). For Maplable it is `https://prod.plan.tools`.

```tsx
import { AmpProvider, AmpWebClient } from '@art-media-platform/web';

const client = new AmpWebClient({
  vaultUrl: import.meta.env.VITE_AMP_VAULT_URL,    // operated node — e.g. https://prod.plan.tools
  planetTag: import.meta.env.VITE_AMP_PLANET_TAG,  // the planet your app reads/writes
});

export default function App() {
  return (
    <AmpProvider client={client}>
      <Router />
    </AmpProvider>
  );
}
```

### Environment Variables

```env
VITE_AMP_VAULT_URL=https://prod.plan.tools   # the operated node (:5193 is only the local-dev default)
VITE_AMP_PLANET_TAG=my-planet-tag

# When reading anonymous shares, point at the share planet:
VITE_AMP_PUBLIC_SHARE_PLANET_TAG=my-planet-tag-shares
```

**Server-side prerequisite for share planets.** `VITE_AMP_PUBLIC_SHARE_PLANET_TAG` resolves anonymously only once the deploy operator performs genesis of a public planet and registers it host-side (`amp planet create`, §6.4). Where that registration is stored is operator-side and substrate-native (§10) — not your concern. Without it, anonymous reads against the planet tag fall through to the standard Bearer gate.

---

## 4. Wire Shape — the Contract

`app.www` listens on port 5193 (the local-dev default; an operated node terminates TLS on 443). Every endpoint accepts `Authorization: Bearer <session-token>`, except where noted. CORS is operator-configured per allowed web origin (§10). The server emits `Content-Type: application/json; charset=utf-8` on JSON responses.

### 4.1 Authentication

```
POST /api/v1/login
  Body: LoginCredentials
  Response: { SessionToken: string, ExpiresAt: number, Member: AmpMember }

POST /api/v1/logout
  Header: Authorization: Bearer <token>

GET  /api/v1/session
  Header: Authorization: Bearer <token>
  Response: { Member: AmpMember, ExpiresAt: number }

GET  /api/v1/me
  Header: Authorization: Bearer <token>
  Response: AmpMember
```

`LoginCredentials` is a discriminated union.  Keys are PascalCase like every
wire field; the `Scheme` *values* stay lowercase (the server dispatches on
them verbatim):

```typescript
type LoginCredentials =
  | { Scheme: 'email';      Email: string; Password: string }
  | { Scheme: 'memberToken'; MemberToken: string }
  | { Scheme: 'yubikey';    ChallengeResponse: string }
  | { Scheme: 'wallet';     Address: string; Signature: string; Nonce: string }
  | { Scheme: 'did';        DID: string; Signature: string; Nonce: string };
```

The unified `/api/v1/login` is **shipped**: `wallet`, `email`, `did`, and `memberToken` are fully wired and Bearer-issuing — `memberToken` is the challenge-less SSO scheme (present `signed(memberID‖ts)`, verified against the member's seated `MemberEpoch.SigningKey`), **live on a host-bridged node** and returning `501` only on the in-memory dev backend (§14.7) (AD-app-www §3.3). `yubikey` still parses cleanly and returns HTTP 501 with `Code: "Unsupported"` until it lands — SDK clients can lock the contract today, and it flips on without any wire-shape change.  Non-2xx responses throw a typed `AmpError` carrying the wire `Code` (surfaced as `AmpError.code`, e.g. `AmpErrorCode.Unsupported`) plus the HTTP `status`, so a client can dispatch on the code and treat a not-yet-wired scheme as a no-op. (Method errors are the exception: a `405` rides `Code: "BadRequest"`, so branch on `AmpError.status` to tell a wrong method from a malformed body.) The cookie-bound path at `/api/v1/login/wallet/{challenge,verify,session,logout}` serves browser flows that prefer per-step cookie handling; both paths share one session store.

**DID scheme (W3C DID 1.0 — login only).** `did` proves control of the key a DID URI names: fetch a challenge with `?did=<uri>`, sign it, and submit `{ Scheme: 'did', DID, Signature, Nonce }`.  Shipped methods: **`did:key`** (Ed25519) and **`did:pkh:eip155`** (Ethereum wallet).  A `did:pkh:eip155:*:0x…` login folds to the *same* MemberID as a `wallet` login over that address (`eth:lc(addr)`) — two URI spellings of one key, one member.  A DID whose method/curve isn't wired yet (e.g. `did:key` P-256/secp256k1, `did:pkh:solana`, `did:web`) returns the same 501 `Unsupported`.  This is DID-Auth — Verifiable Credentials (issuer-signed claims) are out of scope.

**Email scheme additionally exposes recovery + admin-issue endpoints:**

```
POST /api/v1/login/email/recover     (anonymous; returns 202 uniformly)
POST /api/v1/login/email/redeem      (anonymous; consumes token, mints session)
POST /api/v1/admin/credentials/email/issue   (Bearer; admin-gated signup)
```

MemberID for the email scheme = `tag.NameFrom("email:lc(addr)").ID` — mirror of the wallet path's `tag.NameFrom("eth:lc(addr)").ID` rule.  Failure modes (unknown email, wrong password) return a single 401 envelope with timing-uniform KDF cost so the response carries no existence oracle.  Bulk email (campaigns, white-label outreach) routes through `app.email`'s queue channel.

```typescript
interface AmpMember {
  ID: string;                  // member tag.UID, base32
  DisplayName: string;
  Email?: string;
  PlanetID: string;            // planet tag.UID, base32
  Kind?: string;               // tag.UID resolving to a LawMemberKind_*. Default: Person.
  Address?: string;            // 0x-prefixed; present for wallet-scheme members
}
```

### 4.2 Items (CRDT Read)

```
GET  /api/v1/channels/{channel}/attrs/{attr}/items
     ?after=<itemID>&limit=<n>&planetTag=<canonic|UID>
     Response: { Items: Item[], HasMore: boolean, Next?: string }

GET  /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}
     ?planetTag=<canonic|UID>
     Response: Item
```

URL query params (`?after=`, `?limit=`, `?planetTag=`) stay lowerCamelCase —
they are URL params, not JSON body fields.

`channel` and `attr` in the URL path can be either canonic names (`projects`, `labels`) or pre-resolved base32 UIDs — the server parses through `tag.Parse` so both forms work. `planetTag` defaults to the session's bound planet; pass it explicitly to read from the deploy's share planet (anonymous reads — see §6.4) or any other reachable planet.

```typescript
interface Item {
  _ItemID:    string;           // tag.UID, base32
  _EditID:    string;           // latest edit's tag.UID
  _FromID:    string;           // author member ID (only present if reader has access)
  _UpdatedAt: string;           // ISO-8601 — derived from item's tag.UID
  Value:      any;              // the application payload
  _Withdrawn?: WithdrawNote;    // present if a Withdraw cites this item — see §7
}
```

`_UpdatedAt` is `tag.UID`-ordered, not strictly clock-monotonic across hosts. Two writers on different hosts may produce items with identical `_UpdatedAt` strings; LWW tiebreak is by lexicographic compare of `_ItemID`. The list endpoint returns items sorted by `_ItemID` (tag.UID byte order); paginate with `?after=<last-itemID>`.

Cross-planet reads ride the same path with `?planetTag=<other-planet>`; the server resolves the canonic name via `tag.Parse` (decodes base32 UIDs verbatim instead of re-hashing them).

### 4.3 Items (CRDT Write — TxMsg-shaped, batchable)

`POST /api/v1/tx` is the canonical write endpoint. **One request = one TxMsg = one atomic transaction carrying N ops.** A debounced save of 10 entity edits is one TxMsg with 10 ops, not 10 TxMsgs. The single-op verbs below are convenience sugar that compile to a one-op TxMsg.

```
POST   /api/v1/tx
       Body: { Ops: TxOp[], PlanetTag?: string, InvokeURL?: string }   // InvokeURL → verb-RPC (below)
       Response: { TxID, Results: Array<{ ItemID, EditID }> }

TxOp =                                                          // Kind values stay lowercase
  | { Kind: 'create',   Channel, Attr, Value, ItemID? }
  | { Kind: 'upsert',   Channel, Attr, ItemID, Value }
  | { Kind: 'remove',   Channel, Attr, ItemID }
  | { Kind: 'withdraw', Channel, Attr, ItemID, Withdraw: WithdrawNote }   // see §7

POST   /api/v1/channels/{channel}/attrs/{attr}/items                       ─ sugar: tx with one create op
PUT    /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}              ─ sugar: tx with one upsert op
DELETE /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}              ─ sugar: tx with one remove op
POST   /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}/withdraw     ─ sugar: tx with one withdraw op
```

`planetTag` on the canonical `/api/v1/tx` endpoint, and the `?planetTag=<...>` query param on the sugar verbs, target a planet other than the session default — the same way the read endpoints work.

**Atomicity contract.** All ops in a single `POST /api/v1/tx` ride one TxMsg under one encryption context. Batches that span encryption domains (a planet-public op alongside a private-channel op, or two different private channels) are rejected; split into separate `tx` calls.

**WebSocket fan-out.** A 10-op tx produces 10 `update`/`delete`/`withdraw` WebSocket events on the consumer side — one per op. Subscribers receive item events; they don't need to know about transaction boundaries.

**Tombstone semantics.** `remove` writes a tombstone, not a wipe; bytes survive in the journal bound by retention. `withdraw` is the parallel signed signal — see §7.

**Idempotency & retries.** `upsert` / `remove` / `withdraw` are keyed by a caller-supplied `ItemID`, so replaying one (e.g. after a network timeout that swallowed the `TxResponse`) is safe — same key, last-write-wins. A `create` *without* an explicit `ItemID` mints a fresh item on every call, so a blind retry duplicates; supply a deterministic `ItemID` (§5.8) to make `create` idempotent too.

**Verb-RPC — routing a batch to an app handler.** Set `InvokeURL` on `POST /api/v1/tx` to `"amp://~/{app}/{verb}"` (e.g. `amp://~/forums/post`) to route the whole batch to an app's verb handler instead of the default cabinet commit. The host delivers the ops to the named verb as RPC arguments under `PinMode_Invoke` — **not journaled as planet state** — carrying the session member as the tx `FromID`; the app authors any durable writes itself, custodially, recording the invoking member as author. **One batch = one verb**; an empty `InvokeURL` is a normal cabinet commit. This is the write path for a channel a member holds only `Access_ReadOnly` on — the verb, not a direct op, is how the mutation lands. SDK: `client.invoke(verbURL, ops, planetTag?)`, or `invoke` from `useAmpMutation()` (§5.3). (AD-app-forums §3.4.)

### 4.4 Media Upload

```
POST   /api/v1/upload
       Content-Type: multipart/form-data
       Fields: file (required), channel, attr, planetTag (optional)
       Response: amp.Tag (UID + URI + ContentTypeRaw + I/Units=Bytes)

POST   /api/v1/media/resolve
       Body: { PlanetTag?, Blob: amp.Tag }
       Response: amp.Tag (with URI filled by the host's asset publisher)

GET    /www/{UID}
       Response: media stream with Range support, conditional GET, long-cache headers
```

```typescript
// BlobRef mirrors the amp.Tag the server returns (the substrate's universal
// address+meta carrier).  PascalCase keys, base32 UID — one identifier set:
interface BlobRef {
  UID: string;             // blob content hash (leading 16 bytes), base32
  URI?: string;            // server-populated stream URL (/www/{UID})
  ContentTypeRaw?: string; // MIME type (empty ⇒ text/plain); the wire key is
                           // ContentTypeRaw — amp.Tag's raw content-type field
  I?: number;              // plaintext byte length (when Units = Bytes)
  Units?: number;
}
```

**Caller-carries-the-Tag.** The cabinet (channel item that surfaced the BlobRef) is the source of truth for blob metadata.  When you need to render a blob in `<img>`/`<video>`, send the blob's `amp.Tag` (read from the cabinet) to `POST /api/v1/media/resolve`; the host's asset publisher maps it to a streamable `/www/{UID}` URL.  The publisher is in-memory and idempotent — repeated resolves dedupe, vault outage / restart / cross-vault read all just republish on demand.  No cold-store window for filenames or ContentType; no persistent publisher state to migrate.

After upload, write a regular item that references the blob by ID (typically `await upsert(channel, attr, blobRef.UID, { blobRef, ... })`) — the upload endpoint stores the blob bytes; the channel item is the addressable record that points at them.

Encrypted blobs are decrypted on demand by `app.www` using the session's epoch key store; the served bytes are plaintext over the (TLS-protected) wire to the client. Plaintext is never persisted on the vault disk.

### 4.5 WebSocket Subscriptions

```
WS  /ws?token=<sessionToken>
```

Frame format (JSON):

```json
// Client → Server  (Type values stay lowercase)
{"Type": "subscribe",   "Channel": "<id>", "Attr": "<id>"}
{"Type": "unsubscribe", "Channel": "<id>", "Attr": "<id>"}

// Server → Client
{"Type": "update", "Channel": "<id>", "Attr": "<id>",
 "ItemID": "<id>", "Value": {...},
 "EditID": "<id>", "FromID": "<id>", "UpdatedAt": "..."}
{"Type": "delete",  "Channel": "<id>", "Attr": "<id>", "ItemID": "<id>",
 "EditID": "<id>", "FromID": "<id>"}
{"Type": "withdraw","Channel": "<id>", "Attr": "<id>", "ItemID": "<id>",
 "EditID": "<id>", "FromID": "<id>",
 "Withdraw": {                         // WithdrawNote sub-object
   "Reason": "<WithdrawReason>", "Rationale": "...",
   "Subject": "<base32 UID>",                         // omitted when subject == FromID
   "Delegation": "<base32 Address>"                   // omitted when subject == FromID
 }
}
{"Type": "error",   "Channel": "<id>", "Attr": "<id>", "Error": "<reason>"}   // rejected subscribe / bad frame
```

`FromID` is always the TxMsg signer (the member who authored the op). On a `withdraw` frame, `Withdraw.Subject` names whose consent is being withdrawn — equal to `FromID` in the common case (signer is the subject), distinct when an authorized delegate (Memorial, GDPR delegation) speaks on the subject's behalf. `Withdraw.Delegation` is a base32-packed `amp.Address` citing the record proving that authority. `Subject`/`WithdrawnBy` are plain base32 UID strings. See §7.

Subscriptions are per-`(channel, attr)` and deliver every item event on that attr. To scope subscribe by item, partition the data into per-scope attrs at write time (e.g., `widgets/instance.{memberID}` rather than `widgets/instance` filtered by ownerID).

A subscribe the server rejects (no access to the channel/attr, or a malformed key) returns a `{Type:"error", Channel, Attr, Error}` frame on that `(channel, attr)`; the SDK surfaces it as a `{type:'error'}` `SubscriptionEvent` (and `useAmpQuery` sets its `error`) instead of silently never delivering. An error frame is protocol-level, not telemetry.

The server NEVER pushes telemetry frames (failed-login, rate-limit, audit) over the consumer WebSocket. SecurityEvent telemetry is local-only by design.

### 4.6 Planet & federation name resolution (NameService)

> **Three "resolve" verbs — keep them straight.** `/api/v1/tag/resolve` (§5.8) canonizes a *name string* to a `tag.UID` — a pure hash, anonymous, reveals nothing that exists. The NameService `resolve` below maps a registered *FQDN* (`spaces.example.com`) to the *planet* that serves it. The cross-planet `resolve(address)` (§9, lands M5) follows a packed `amp.Address` to one record. Same English word, three layers — don't wire them to the same code path.

NameService is amp's federation directory: it answers "which planet is `spaces.example.com`, and where is its vault dialable?". **`resolve` is anonymous** — it answers a single exact FQDN off the host's federation resolver, so a fresh install or a deep-link source can dial + pin a named planet before it has any session. **`search` and `federation/peers` require `Authorization: Bearer`** — ranked enumeration is membership-gated, so a session walks only the federations it has joined (see Posture).

```
POST /api/v1/resolve                         (anonymous — no Bearer)
     Body: { FQDN }                          → ResolveResponse   (404 if the host's federation resolver has no record)
POST /api/v1/search                          (Bearer)
     Body: { Query, Limit? }                 → { Matches: SearchMatch[] }   (ranked, best-effort)
GET  /api/v1/federation/peers?federation=<base32-UID>   (Bearer)
                                             → { Peers: FederationPeerEntry[] }
```

```typescript
interface ResolveResponse {
  FQDN:          string;
  PlanetID:      string;     // base32 tag.UID — the planet the FQDN names
  AnsweredBy:    string;     // base32 tag.UID — federation that answered
  VaultAddrs:    VaultEndpoint[];   // dialable bootstrap addrs — returned in full by resolve
  TrustState:    'Unchecked' | 'Verified' | 'Refuted';
  PinPrecedence: boolean;
  Ambiguous:     boolean;    // >1 federation claims this FQDN
  Hops:          number;     // forwarding hops to the answer
}
interface VaultEndpoint       { Transport: string; Address: string /* base64 */ }
interface SearchMatch         { PlanetID: string; FQDN: string; AnsweredBy: string;
                                Score: number; AppName: string; AppDesc: string; Platforms?: string[] }
interface FederationPeerEntry { FederationID: string; VaultAddrs: VaultEndpoint[]; Label?: string }
```

**`TrustState` is load-bearing — never silently pick.** A record is `Verified` only when the answering federation matches the planet's own `Brand` back-edge (the planet consents to being named there). `Refuted` flags a third party claiming a name the planet never authorized; `Unchecked` means the back-edge wasn't confirmed. When `Ambiguous` is set or `TrustState != 'Verified'`, surface it and let the user choose — do not auto-follow.

**Posture — `resolve` is anonymous; bulk discovery is gated.** `resolve` answers one exact FQDN off the host's federation resolver with no Bearer, returning that planet's `VaultAddrs` in full so any caller can dial it. FQDN keys are low-entropy and dictionary-reversible, so a private namespace's privacy comes from federation **unreachability** (not a member → can't reach the federation that names it), never from key secrecy. What *is* gated is enumeration:
- **`search` is best-effort discovery over the federations you've joined — Bearer-only, not a scrape endpoint.** Ranked enumeration is the scraping surface, so a session walks only its joined federations. Don't build features that depend on bulk-enumerating the namespace.
- **You don't need NameService to run a single deploy.** Your planet tag is handed to the client at construction (`VITE_AMP_PLANET_TAG`); reads/writes/subscribes never touch it. Resolve/search matter only when you discover *other* planets across federations.

**Publishing a name (operator-side).** A name enters the directory when an operator registers it into a federation's NameService channel — a signed governance write, not a consumer call:

```bash
amp name register <fqdn> --target <planet-UID> [--federation <UID>] [--vault transport:addr …]   # alias: amp ns
```

### 4.7 Joining a federation — invite accept

Accepting an invite is **how you join a federation**, and **federation membership
is the authorization to publish names** (§4.6). You don't need it to read or write
your *own* home planet — that works the moment you log in (§0) — but you **do**
need it to register your planets / names into a federation and be discoverable
across orgs (it mints your member keys on your side). This is the bootstrapping
step for the cross-org / licensee workflow. Log in first (§4.1), then redeem the
token:

```
POST /api/v1/invite/accept            (Bearer)
     Body:     { InviteText: string, Passphrase: string }
     Response: { PlanetID: string, MemberID: string }    // 201 Created; base32 UIDs
```

- `InviteText` is the invite's **universal URL** `https://{fqdn}/invite#{body}`
  (the sealed token rides the URL fragment, so a click never sends it to the
  host) — or its bare amp-base32 body. Whitespace / newline / case-tolerant, so
  it survives copy-paste and transit; a mangled paste fails the CRC cleanly.
- `Passphrase` is delivered **out-of-band**. Generate it from a CSPRNG with **≥128
  bits of entropy** (a diceware phrase or `crypto.getRandomValues`), never a
  human-chosen word: the sealed token is offline-attackable by anyone who captures
  it, so its safety rests on passphrase entropy, not on the token being secret. With
  a strong passphrase the token is inert in transit; with a weak one it is
  brute-forceable — see SECURITY-amp-web-SDK.md.
- Returns `501` on the in-memory dev backend (§14.7); live on a host-bridged node
  such as `prod.plan.tools`.
- Issuing invites is the operator side of this — `POST /api/v1/invite/issue` (§14.4).

From the SDK:

```ts
const { PlanetID, MemberID } = await client.acceptInvite({
  inviteText: 'https://{fqdn}/invite#…',   // the universal URL, or its amp-base32 body
  passphrase: '…',                          // out-of-band
});
// You are now a member of that federation's planet; from here you can register
// your own names with `amp name register` (§4.6).
```

**Issuing, revoking, and listing invites** (operator side; Bearer, host-bridged):

```
POST /api/v1/invite/issue      Body { Planet, Passphrase, MaxRedemptions?, Access?, ExpiresAt?, VaultAddrs? }
                               Response { PlanetID, InviteID, InviteText }   // 201
POST /api/v1/invite/revoke     Body { Planet, InviteID? | InviteText?, Rotate? }   // 200
GET  /api/v1/invite/list?planet={UID}    Response { Policies: [{ InviteID, MaxRedemptions, Status, ExpiresAt?, Redemptions:[{ Member, RedeemedAt, Rank, InRank }] }] }
```

`MaxRedemptions` selects the invite kind: **0** (or omitted) mints a single-use
pre-minted slot; **> 0** mints a multi-use self-mint policy that many members
join off one URL, each minting their own identity. `Access` (`ReadWrite`, …) is
what each redeemer is granted; `ExpiresAt` is unix seconds (0 = the planet's
bootstrap TTL). `revoke` is **terminal** (reissue, never un-revoke); `Rotate`
also rotates the planet epoch to retire the token-held key (node-custodial
founder only). `list` shows each policy's redemption ledger with an adjudicated
`Rank` — a record with `InRank: false` is over the ceiling and void.

```ts
const { InviteID, InviteText } = await client.issueInvite({
  planet, passphrase, maxRedemptions: 25, access: 'ReadWrite',
});
const { Policies } = await client.listInvites(planet);
await client.revokeInvite({ planet, inviteId: InviteID, rotate: true });
```

Bootstrapping with no SDK code yet? **The Bearer is your login `SessionToken`**
(§4.1). With the SDK you never handle it — `client.login()` stores it and
`client.acceptInvite()` sends it; for a raw curl, read `SessionToken` from a login
response into `$AMP_TOKEN`:

```bash
# Wallet (SIWE) needs a signed challenge — easiest via the SDK login, which then
# holds the session for you; the email scheme is a plain POST once provisioned.
AMP_TOKEN=$(curl -sX POST https://prod.plan.tools/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"Scheme":"email","Email":"you@org","Password":"…"}' | jq -r .SessionToken)

curl -sX POST https://prod.plan.tools/api/v1/invite/accept \
  -H "Authorization: Bearer $AMP_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"InviteText":"https://prod.plan.tools/invite#…","Passphrase":"…"}'
```

---

## 5. `@art-media-platform/web` — React API Reference

Thin wrappers over §4. Hooks share a single `AmpWebClient` via `<AmpProvider>`.

### 5.1 `useAmpAuth()`

```tsx
const { member, login, logout, isAuthenticated, loading } = useAmpAuth();

await login({ Scheme: 'email', Email: email, Password: password });
await login({ Scheme: 'wallet', Address: address, Signature: signature, Nonce: nonce });
await login({ Scheme: 'did', DID: did, Signature: signature, Nonce: nonce });
```

> Imperative calls below — `getWalletChallenge`, `getDIDChallenge`, `resolveTag` — live on the adapter. Get it in any component with `const client = useAmpClient();`.

**Wallet sign-in (SIWE + multi-wallet).** The `'wallet'` scheme is **EIP-4361 (Sign-In with Ethereum)** over any EVM wallet. Discover the user's wallet(s) with **EIP-6963** so the picker shows each by name + icon — the brand lives in the UI, never on the wire (MetaMask, Coinbase, Rainbow, … all flow through the same `personal_sign`):

```tsx
const client    = useAmpClient();   // adapter for the challenge fetch below
const providers = new Map();
window.addEventListener('eip6963:announceProvider', (e) => providers.set(e.detail.info.rdns, e.detail));
window.dispatchEvent(new Event('eip6963:requestProvider'));   // each wallet announces { info: { name, icon, rdns }, provider }

// user picks `provider` from the list (render info.name + info.icon), then:
const [address] = await provider.request({ method: 'eth_requestAccounts' });
const challenge = await client.getWalletChallenge(address);   // server builds the domain-bound SIWE message
const signature = await provider.request({ method: 'personal_sign', params: [challenge.Message, address] });
await login({ Scheme: 'wallet', Address: address, Signature: signature, Nonce: challenge.Nonce });
```

The host renders the EIP-4361 message bound to its own domain (anti-phishing); `getWalletChallenge` takes the address so the canonical SIWE serialization stays server-side.

**DID sign-in (`did:key` Ed25519 / `did:pkh:eip155`).** Same challenge → sign → submit shape, requesting the challenge by DID instead of address:

```tsx
const challenge = await client.getDIDChallenge(did);   // GET …/login/challenge?did=<uri>
const signature = await signChallenge(did, challenge.Message);  // ed25519 for did:key; personal_sign for did:pkh:eip155
await login({ Scheme: 'did', DID: did, Signature: signature, Nonce: challenge.Nonce });
```

For `did:pkh:eip155` the signer is the same EVM-wallet `personal_sign` as above (the DID just carries the address), and the resulting member is identical to the `wallet` path. For `did:key` the signer is whatever holds the Ed25519 private key (client-custody is an open UX question).

### 5.2 `useAmpQuery<T>()`

```tsx
const { data, loading, hasMore, loadMore, refetch } =
  useAmpQuery<ProjectSnapshot>('projects', 'snapshot', {
    limit: 50,
    orderBy: '_UpdatedAt',
    filter: { ownerID: member.ID },
  });
```

The hook subscribes via WebSocket automatically and re-renders on updates. `data[i]` carries `ItemMeta` fields underscore-prefixed.

### 5.3 `useAmpMutation()`

```tsx
const { tx, invoke, create, upsert, remove, withdraw, loading } = useAmpMutation();

// One TxMsg, many ops — atomic, single signature, single MemberProof:
const results = await tx([
  { Kind: 'upsert', Channel: 'projects', Attr: 'labels',   ItemID: l1, Value: lv1 },
  { Kind: 'upsert', Channel: 'projects', Attr: 'labels',   ItemID: l2, Value: lv2 },
  { Kind: 'upsert', Channel: 'projects', Attr: 'articles', ItemID: a1, Value: av1 },
  { Kind: 'remove', Channel: 'projects', Attr: 'circles',  ItemID: c1 },
]);

// Verb-RPC — route the batch to an app handler instead of a cabinet commit (§4.3).
// The write path for a channel you hold only Access_ReadOnly on; the app authors custodially:
const posted = await invoke('amp://~/forums/post', [
  { Kind: 'create', Channel: 'threads', Attr: 'posts', Value: { body, threadID } },
]);

// Single-op convenience wrappers (each is one one-op tx under the hood):
const itemID = await create('projects', 'snapshot', value);   // the new item's ID, or '' if the write produced no result
await upsert('users', 'profile', member.ID, patch);
await remove('projects', 'snapshot', itemID);
await withdraw('shares', 'link', itemID, {
  reason: 'Departed',
  rationale: 'left the team',
  // subject + delegation are optional — omit when the signer is the subject.
  // (WithdrawOpts keys are camelCase — an SDK option bag, never serialized.)
  subject: deceasedMember.ID,                            // delegated path — a delegate withdraws on the subject's behalf
  delegation: delegationAddress,                         // base32-packed amp.Address
});
```

`tx(ops)` is the canonical write — all ops in one batch ride a single TxMsg, sealed under a single encryption context, with one signature and one MemberProof. A debounced project save with 50 entity changes is **one** TxMsg, not 50. Mixing encryption domains in one batch (planet-public alongside private-channel ops) is rejected — split into separate `tx()` calls.

`invoke(verbURL, ops)` posts the same op batch to an app verb handler (`amp://~/{app}/{verb}`) instead of committing to a cabinet — the write path for channels you hold only `Access_ReadOnly` on, where the app authors the durable write custodially (§4.3).

`upsert` accepts any client-supplied `tag.UID` for the item. For singleton items, derive a stable UID from a well-known name — generate it at build time with `forge` or resolve it once via `client.resolveTag('settings:theme-preference')` (§5.8) — or reuse the member's own UID directly. The `scheme:identifier` form matches CAIP-10 / DID conventions.

### 5.4 `useAmpUpload()`

```tsx
const { upload, progress, uploading } = useAmpUpload();

const blobRef = await upload(file, 'projects', {
  attr: 'media',
  metadata: { caption, tags: ['vacation'] },
});
// store blobRef.UID inside an item:
await upsert('projects', 'media', blobRef.UID, {
  blobRef,
  filename: file.name,
});
```

### 5.5 `useAmpMedia()`

```tsx
const { url, loading, contentType, byteSize } = useAmpMedia(blobUID);
// url is /www/{UID}; pass to <img>, <video>, <audio>, or download <a>
```

### 5.6 Sealed-box helpers (§6.2)

```tsx
const { seal, open } = useAmpCrypto();

const sealedBytes = await seal(plaintext);            // anonymous-sender to self
const plaintext   = await open(sealedBytes);
```

### 5.7 Cross-planet Address

```tsx
// An amp.Address rides the wire as a single opaque base32 string packing 3–5
// UIDs (element / +edit / +planet).
// The SDK treats it as opaque: pass through the string the server produced.
const addr = addressFromServer;   // a base32 string — use it as-is

// Reading a planet you can reach (a public share, or a cross-planet record) is a
// planet-scoped query today — pass planetTag to query.  The one-call
// resolve(address) REST primitive lands at M5:
const { data } = await client.query(channel, attr, { itemID, planetTag: planetID });
```

### 5.8 Deterministic UIDs — names → `tag.UID`

`(channel, attr, itemID)` are `tag.UID`s derived from string names by amp's `tag.Name` canonization — a regex split on whitespace + punctuation, a case-insensitive lowercase ASCII fold (A–Z → a–z; non-ASCII bytes verbatim), URL-trigger-char (`:` / `/` / `\`) handling, and an atomic word fold (word order is significant). **Don't reimplement this in JS / Swift / C#** — a subtly-wrong port yields UIDs that 404 on attrs that "should exist," and the failure stays invisible until a write lands in the wrong place. Two first-class ways get the right UID without porting the algorithm:

#### Build-time — `forge` codegen (preferred for well-known names)

Generate canonic `TagName` / `TagUID` constants from your `.sdl` keys with [`forge`](https://github.com/art-media-platform/forge), the same codegen amp uses internally (Go, C#, **TypeScript**, Python, and C targets). Your app then references compile-time constants — no runtime round-trip, no registering known keys at boot, and no risk of a hand-typed name drifting from the server's UID:

```bash
go run github.com/art-media-platform/forge/cmd/forge consts your.keys.sdl \
  --ts_out ./src/amp-consts.ts
```

#### Runtime — `resolveTag` / `resolveTags` (server canonization)

For names you only learn at runtime — or to warm a cache from the deploy's published catalog — resolve them server-side. The server applies the canonical `tag.Name` rule and returns the UID: one authoritative answer for every binding.

```tsx
// Bootstrap: resolve every well-known name your app uses once, then cache.
const named = await client.resolveTags([
  'projects.labels',
  'projects.polygons',
  'users.profile',
  `widgets.instance.${memberID}`,
]);
// named: { Expr, Canonic, ID }[] — cache in IndexedDB; reuse across sessions.
```

Anonymous and long-cacheable (`Cache-Control: public, max-age=86400`):

```
GET  /api/v1/tag/resolve?expr=amp.member.profile   → { Expr, Canonic, ID }
POST /api/v1/tag/resolve   Body: { Exprs: [...] }   → { Results: [{ Expr, Canonic, ID }, ...] }
```

**Already holding a UID?** A base32 `tag.UID` — 26 digits, rendered grouped `6-5-5-5-5` with `-` separators (e.g. `member.ID`, or an `itemID` from a read) — is already in wire form — pass it straight through. Decoding strips `-` and whitespace, so the grouped and solid forms resolve to the same UID. `resolveTag` round-trips a UID back to the same UID rather than re-hashing it, so mixing names and UIDs in one `resolveTags` batch is safe.

---

## 6. Data Model Conventions

### 6.1 Channel and Attribute Naming

- `(channel, attr)` is a logical bucket addressing CRDT items via `tag.UID`. Both names are tag.UIDs derived from string identifiers (forge-keycomb generated).
- Well-known names live in `amp.std.consts.sdl` under `amp.law/*`, `amp.ledger/*`, `amp.member/*`, `amp.home/*`, `system.property/*`, `amp.blob/*`. App-specific names are added in the app's own `consts.sdl` and regenerated via `make generate`.
- App-specific names should use a deploy-prefix (e.g. `maplable.projects` rather than bare `projects`) to avoid collision with future shared names.

**Maplable's channel layout (the worked example throughout this SKILL):**

| Maplable concept | channel | attr | itemID convention | Notes |
|---|---|---|---|---|
| Project full state | `projects` | `snapshot` | server project UID | Per-entity item split recommended; see §6.3 |
| Project listing metadata | `projects` | `meta` | same as snapshot itemID | `{ name, thumbnail, _UpdatedAt, ownerID, templatePlanet }` |
| Project share state | `projects` | `share` | same itemID | `{ isPublic, shareUrl, sharedAt, viewCount }` |
| User profile | `users` | `profile` | member UID | `{ displayName, firstName, lastName, theme, accentColor }` |
| User defaults | `users` | `defaults` | member UID | per-field `{value, ts}` for LWW |
| User BYOK keys | `users` | `api_keys_overrides` | member UID | **sealed bytes only — see §6.2** |
| Widget instances | `widgets` | `instance.{memberID}` | uuid | per-member partition for clean subscribe |
| Project templates | `templates` | `definition` | uuid | admin write; member read |
| Shareable view links | `shares` | `link` | short slug | written on a separate public planet — see §6.4 |

### 6.2 Storing user-supplied secrets (BYOK)

User-supplied API keys (Cesium Ion, OpenRouter, Mapbox, etc.) **must not** land in plaintext channel items. Channel items are readable by every member of the planet who holds the planet epoch key, and a memory scrape of the local epoch key cache exposes them post-hoc. The right pattern:

Channel item values are JSON, so the sealed bytes go on the wire as a base64
string (a raw `Uint8Array` does not survive `JSON.stringify`).  Use the
`bytesToBase64` / `base64ToBytes` helpers on the way in and out.

```tsx
import { useAmpMutation, useAmpCrypto, bytesToBase64 } from '@art-media-platform/web';

function ApiKeysForm() {
  const { upsert } = useAmpMutation();
  const { seal } = useAmpCrypto();

  async function saveKey(slot: 'cesium' | 'openrouter' | 'mapbox', plaintext: string) {
    const sealed = bytesToBase64(await seal(new TextEncoder().encode(plaintext)));
    // Read-modify-write: each slot is sealed independently
    const current = await getApiKeysItem();          // returns Record<slot, base64 string>
    const next = { ...current, [slot]: sealed };
    await upsert('users', 'api_keys_overrides', member.ID, next);
  }
}
```

```tsx
import { useAmpCrypto, base64ToBytes } from '@art-media-platform/web';

async function useCesiumIonToken() {
  const { open } = useAmpCrypto();
  const { data } = await client.query<{ cesium?: string }>('users', 'api_keys_overrides', { itemID: member.ID });
  const item = data[0];
  if (!item?.cesium) return null;
  const plaintext = new TextDecoder().decode(await open(base64ToBytes(item.cesium)));
  // Use it for one outbound request; don't persist outside this scope.
  return plaintext;
}
```

The `seal/open` primitives wrap `safe.Encrypt.Seal` / `safe.Encrypt.Open` against the session member's `EncryptKey` — anonymous-sender HPKE base mode. The sealed bytes are opaque to anyone but the sealing member, including admins, vault relays, other planet members, and even a future memory snapshot of `eks.keys`.

That `EncryptKey` is **device-local and auto-managed**: the client generates it on first login and persists it in browser storage (IndexedDB), then installs it on every later login — so `seal`/`open` work for any logged-in member with no setup. Because the private key never leaves the device, scope is **same-device**: a member who clears storage or signs in on another device re-derives a fresh key there and re-enters their (re-enterable) BYOK secrets. Cross-device "seal on phone, open on laptop" is a deliberate non-goal of this model — see `SECURITY-amp-web-SDK.md`. Surface the trade-off in your UI so it isn't a surprise — e.g. label a BYOK field *"Stored on this device only — re-enter it when you sign in elsewhere."*

The default kit is **Poly25519** (X25519 + XChaCha20-Poly1305 + HKDF-SHA256) — pure JS via `@noble/curves` + `@noble/ciphers` + `@noble/hashes` (the SDK's only runtime dependencies), no WASM. It is the **only kit in the default bundle**; P-256 (YubiKey-attached members) and secp256k1 (crypto-wallet members) register when their auth schemes ship and are absent until then. Bundle-minimization is **transparent** — there is no separate entry point or build flag to set: importing `@art-media-platform/web` and sealing BYOK pulls Poly25519 plus the three `@noble/*` packages and nothing more, so a card or widget that only seals secrets stays small.

Envelope layout, byte-compatible with `safe.KeySpec.Encrypt.Seal/Open` Go-side:

```
seal output = eph_pub (32) || nonce (24) || ciphertext+tag   // Poly25519
```

A payload sealed in TS opens cleanly Go-side and vice versa. The local round-trip is locked in `src/crypto/poly25519.test.ts`; byte-level interop is locked bidirectionally in `src/crypto/interop.test.ts` and `stdlib/safe/poly25519/poly25519_interop_test.go`, which open the same Go-sealed and TS-sealed vectors on both sides.

For vanilla-JS consumers without React, the same surface is reachable as client methods:

```ts
const client = new AmpWebClient({ vaultUrl, planetTag });
await client.login({ Scheme: 'email', Email: email, Password: password });
// login() auto-installs the member's device-local EncryptKey, so seal/open
// are ready here.  Call client.setEncryptKey(...) only to override with a key
// sourced elsewhere; null on logout.

const sealed = await client.seal(new TextEncoder().encode(plaintext));
const plain  = await client.open(sealed);
```

The lower-level `seal(plaintext, recipientPubKey)` / `open(sealed, recipientKeyPair)` exports cover the case where the consumer sealed for another member (e.g. an address handoff).

Where the third-party API supports it (OpenRouter scoped tokens, Stripe Connect customer-scoped keys), prefer split-capability shape: the secret stays on the device, the device exchanges it for a derived per-call capability, and only the capability rides the wire.

### 6.3 Item size discipline

`PlanetEpoch.MaxTxMsgSize` defaults to 4 MB. Large monolithic JSON values (a multi-megabyte project snapshot blob) replicate the entire payload on every save and burn quota fast. Two corrections:

- **Split by entity bucket.** Prefer `projects/labels/{itemID}`, `projects/articles/{itemID}`, etc., over a single `projects/snapshot/{itemID}` blob. The CRDT-merge surface narrows to the changed entities; cross-device subscribe fires per change.
- **Move binaries to BlobRef.** Anything larger than ~64 KB belongs on the BlobRef path (§4.3), not inline JSON. The item references `blobRef.UID` and the blob streams via `/www/{UID}`.

### 6.4 Public reads & sharing

amp does NOT have a per-item visibility flag. Public-readable content lives on a separate planet operating in public mode (`PlanetEpoch.IsPublic = true` at genesis — see `SECURITY-amp-web-SDK.md`).

#### Operator setup (one time per deploy)

The deploy operator performs share planet genesis once and registers it with the running `ampd` host:

```bash
# After wallet-login via the web (POST /api/v1/login scheme=wallet),
# pass the resulting Bearer token to the CLI:
AMP_TOKEN=<bearer> amp planet create --tag <name>
# → { "PlanetID": "<base32 UID>", "Tag": "<canonic>", "Public": true }
```

The host then records that planet as anonymous-readable so the registration survives restarts. Where that fact is stored is operator-side and substrate-native — see §10; it is not something your app ships or reads.

#### Anonymous read URL shape

Once registered, anonymous reads against the share planet skip the Bearer gate:

```
GET /api/v1/channels/{channel}/attrs/{attr}/items?planetTag=<share-name-or-UID>
GET /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}?planetTag=<share-name-or-UID>
```

Either form (canonic name or base32 UID) resolves to the same planet via `tag.Parse`. Writes (POST/PUT/DELETE/withdraw) against the share planet still require Bearer — anonymous reads, member writes.

#### Sharing flow

1. Owner clicks Share on a project. The web app copies (or canonical-forks) the project's `snapshot` from the owner's planet onto the share planet. Owner attribution is stripped by default; the owner can opt-in.
2. The share planet's `shares/link` item carries the slug → public-snapshot pointer.
3. Anonymous viewers read the share planet via the channels-prefixed path above without `Authorization`. Only the share planet allows anonymous reads — the owner's main planet still requires auth.
4. To unshare, the owner posts a `Withdraw` against the share planet's snapshot item; the public bytes survive in the share planet's journal but the consumer sees both the original and the withdrawal — see §7.

A whitelabel deploy registers two planet tags in its config: the owner planet and the share planet. Each `AmpWebClient` binds one default `planetTag`; read another reachable planet by passing `planetTag` to `query` — anonymous against a registered share planet, authenticated otherwise.

### 6.5 Identity & member shape

`AmpMember` carries:
- `ID` — the member's `tag.UID` on this planet.
- `DisplayName` — auto-generated callsign or user-set.
- `Email?` — optional, only present when the auth scheme exposes it.
- `PlanetID` — the planet this member is scoped to.
- `Kind?` — `tag.UID` resolving to a `LawMemberKind_*` definition. Default `LawMemberKind_Person`. Apps surface this in UI but do NOT gate behavior on it; communities decide what each Kind can do via per-channel ACC, not via protocol-level rules.
- `Address?` — 0x-prefixed wallet address; present for wallet-scheme members.

A given human can have many `MemberTag`s — one per planet they belong to. Cross-planet equivalence is an Equivalence claim, not an automatic merge.

---

## 7. Soft delete & withdrawal

The chronicle is immutable. `DELETE /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}` writes a tombstone — a TxOp that the resolved-state view interprets as "no longer authoritative." Bytes survive in the journal bound by retention.

For records about a member that the member wants disowned (a project shared on a public planet, a profile attribution), the right primitive is **Withdraw** — a parallel signed signal. `withdraw('shares', 'link', itemID, { reason, rationale })` writes a Withdraw record. The original item stays signed and visible; consumers see both the original and the withdrawal, and choose honoring policy (suppress, redact, recontextualize). Withdraw reasons (the `WithdrawReason` union — the wire value is the bare enum name) are `Consent`, `Inaccuracy`, `Outdated`, `Coerced`, `Forgotten`, `Departed`, `InviteRecall`, `Retracted`. A reason is required — a withdraw with no reason is rejected, so there is no "unspecified" value in the union.

### 7.1 Wire surface

A `withdraw` op carries the full withdrawal information set:

```typescript
// WithdrawOpts is the SDK option bag (camelCase — never serialized directly;
// the client maps it to the wire shape):
interface WithdrawOpts {
  reason:     WithdrawReason;     // one of the WithdrawReason values (bare string)
  rationale?: string;             // free-text; for human / future-being readers
  subject?:   string;             // base32 member UID — defaults to signer if omitted
  delegation?: string;            // base32-packed amp.Address; proves authority when subject != signer
}
```

On reads, the withdrawal comes back as the `_Withdrawn` companion alongside the
original Item — wire shape, PascalCase:

```typescript
interface WithdrawNote {
  Reason:      WithdrawReason;
  Rationale?:  string;
  WithdrawnAt?: string;           // ISO-8601 — server-observed timestamp
  WithdrawnBy?: string;           // signer's member UID, base32
  Subject?:    string;            // who the withdrawal speaks for, base32
  Delegation?: string;            // base32-packed amp.Address proving authority
}
```

`Subject == WithdrawnBy` is the common case — the signer is the subject. They differ only when an authorized delegate (Memorial speaking for a deceased member, GDPR delegation, etc.) issues the withdrawal on the subject's behalf; then `delegation` cites the record that grants that authority.

### 7.2 UX distinctions

| Verb | Maps to | Used for |
|---|---|---|
| Delete | `remove` | "Remove this from my project list" — owner removes their own record |
| Unshare | `withdraw` | "Take down the public copy" — record stays signed; consumers see the withdrawal |
| Forget me | `withdraw` with `Reason=Forgotten` | "Right-to-be-forgotten" request; per-jurisdiction node operators may also delete bytes locally to comply with law |
| Memorial unshare | `withdraw` with `subject` + `delegation` | An authorized delegate withdraws on behalf of someone who can no longer act for themselves |

A query for an item returns the latest version AND any withdraw signals citing it. Consumers MUST surface both.

---

## 8. Cards & WebRect-hosted forms

A **card** is a small, self-contained HTML document that renders inside a Unity `WebRect` Pane (3D/XR) or as a 2D drawer in a desktop browser. The same HTML works in both. Cards are how amp builds forms, item-detail views, AI-generated UI, and partner widgets.

### 8.1 Cards are detail views, not list-renderers (the activate-to-open rule)

A card is the detail view of a **single item**, opened on activation. Lists are rendered natively by the host — Cesium glyphs on a map, native rows in a Unity scroll view, DOM elements in a browser SPA — drawn from the underlying CRDT items. Tapping an item opens its card in a Pane.

Concretely, a Maplable map with 300 labels does NOT render 300 WebRect cards; it draws 300 native Cesium pins from `projects/labels/{itemID}` items, and tapping one opens its detail card in a single Pane. WebView count typically stays ≤5 active at a time. The same rule applies to Unity inventory grids, contact lists, file browsers, and any other dense surface — render the list natively, open the card on activate.

This keeps the card pattern viable on phones, XR headsets, and embedded surfaces where 100+ WebViews would be a non-starter.

### 8.2 Why cards are web HTML, not native

- AI agents generate cards in seconds; native UI takes weeks.
- Forms, inputs, file pickers, and Stripe checkout work natively in HTML.
- A card deployed once renders identically on phone, tablet, desktop, and XR.
- Maplable widget cards, Tunr playlist cards, and third-party partner cards all sit inside the same Pane mechanism with the same affordances.

> **Tunr** is a separate amp app — a music / playlist surface — that happens to render its UI as cards. It is **not** the WebRTC **TURN** relay protocol; the names collide but are unrelated. Your app's own WebRTC/TURN usage is independent of anything in this SDK.

### 8.3 Card lifecycle

```
1. Host (Unity or browser) opens a Pane on item activation:
   {vaultUrl}/cards/{card-type}?token=<sessionToken>&planet=<planetTag>&...
2. app.www serves the HTML.  The card's <head> declares its title, intents,
   sized hints, and focus model via <meta name="amp:card:*"> tags.
3. The Pane reads the manifest, configures the WebView, and exposes window.amp.
4. The card calls amp.read / amp.write / amp.subscribe / amp.upload to bind
   to amp state.  Live updates from cross-device edits land via amp.subscribe.
5. The card handles forms via amp.submit; back-nav via amp.back.  The Pane
   responds to amp.navigate / amp.setTitle / amp.setVar.
6. On Pane close, the card receives a beforeunload event; the WebView
   cleans up.
```

### 8.4 Card manifest (HTML head)

```html
<meta name="amp:card:title"        content="Add Label">
<meta name="amp:card:intents"      content="form,navigation">
<meta name="amp:card:size:hint"    content="320x640">
<meta name="amp:card:focus:model"  content="trap">       <!-- trap | passthrough -->
<meta name="amp:card:back:handles" content="true">       <!-- card consumes back -->
```

The Pane introspects these on load. Manifests survive AI generation because they're standard meta tags.

### 8.5 `window.amp` Bridge IDL

```typescript
interface AmpBridge {
  // ── Identity ──
  member: { ID: string; DisplayName: string; PlanetID: string; Kind?: string } | null;

  // ── Data ──
  read(channel: string, attr: string, itemID: string): Promise<any>;
  list(channel: string, attr: string, opts?: ListOpts): Promise<any[]>;
  tx(ops: TxOp[]): Promise<TxReceipt>;         // batched write — one TxMsg, N ops
  // TxReceipt = { txID: string; accepted: boolean } — "transmission received; queued for delivery + processing"
  write(channel: string, attr: string, itemID: string, value: any): Promise<void>;  // sugar: tx with one upsert
  remove(channel: string, attr: string, itemID: string): Promise<void>;             // sugar: tx with one remove
  withdraw(channel: string, attr: string, itemID: string, opts: WithdrawOpts): Promise<void>;
  subscribe(channel: string, attr: string, cb: (event: SubscriptionEvent) => void): () => void;

  // ── Media ──
  upload(channel: string, opts?: UploadOpts): Promise<BlobRef>;
  resolveMedia(blob: BlobRef): Promise<BlobRef>;     // returns blob with URI (stream URL) set

  // ── Sealed secrets ──
  seal(plaintext: Uint8Array): Promise<Uint8Array>;
  open(sealed: Uint8Array): Promise<Uint8Array>;

  // ── Card navigation ──
  navigate(cardUrl: string): void;            // push next card on the stack
  back(): void;                               // pop current card
  setTitle(title: string): void;              // card tells Pane its title
  focus(elementID: string): void;             // request keyboard focus
  onBack(cb: () => boolean): void;            // intercept back; return true to consume
  onFocusChanged(cb: (hasFocus: boolean) => void): void;
  onScroll(cb: (delta: number) => void): void;  // iPod-wheel input

  // ── Form submission ──
  submit(form: FormPayload): Promise<SubmitResult>;
  // FormPayload = { intent: string; values: Record<string,any>;
  //                 channel?, attr?, itemID? }
  // SubmitResult = { ok: boolean; itemID?: string; error?: string }

  // ── Live vars (Card ↔ Pane) ──
  setVar(key: string, value: any): void;
  onVar(key: string, cb: (value: any) => void): void;
}
```

`AmpBridge` and its helper types (`ListOpts`, `TxReceipt`, `FormPayload`, `SubmitResult`) ship in the SDK — `import type { AmpBridge } from '@art-media-platform/web'` for card-author autocomplete, and importing the package augments `window.amp` on the global `Window`. The host (Unity WebView, browser shim, or test harness) injects the implementation. A standalone-browser fallback logs every call to console for debugging:

```javascript
const amp = window.amp || {
  read: (...a) => (console.log('amp.read', a), null),
  write: (...a) => console.log('amp.write', a),
  subscribe: () => () => {},
  navigate: (url) => console.log('amp.navigate', url),
  // ...
};
```

#### Server route — `/cards/{cell}[/asset]`

`app.www` serves card bundles at `/cards/{cell}` (anonymous; no Bearer required). Each cell is a flat directory containing `index.html` and any sibling assets. The canonical cells ship embedded in the `ampd` binary; an operator drop at `{homePath}/cards/{cell}/` shadows the embedded copy for whitelabel customization without a rebuild.

Cell names match `[A-Za-z0-9_-]+` and the filesystem is the registry (no allowlist). `/cards/{cell}` and `/cards/{cell}/` both resolve to `index.html`. Path traversal returns 404. Non-GET/HEAD verbs return 405.

Every response carries a strict CSP that locks the cell to its own origin:

```
default-src 'none';
script-src 'self';
style-src 'self' 'unsafe-inline';
img-src 'self' data: blob:;
font-src 'self' data:;
connect-src 'self';
frame-ancestors 'self';
base-uri 'none';
form-action 'self'
```

Inline `<script>` is forbidden — JS ships as a sibling file (`card.js`). Inline `<style>` blocks are permitted for skinning convenience. `postMessage` is the bridge to the parent frame and is not gated by CSP.

The canonical example cell (`cards/example/`, shipped embedded in `ampd`) exercises every Bridge IDL verb and is the recommended starting template.

### 8.6 Card template

A card is a small sibling-file bundle: `index.html` references `card.css` and `card.js` as siblings. No bundler step; the bundle is the deploy unit.

```
cards/settings/
  index.html
  card.css
  card.js
```

```html
<!-- index.html -->
<!DOCTYPE html>
<html>
<head>
  <meta name="viewport" content="width=device-width, initial-scale=1, user-scalable=no">
  <meta name="amp:card:title"        content="Settings">
  <meta name="amp:card:intents"      content="form">
  <meta name="amp:card:size:hint"    content="320x640">
  <meta name="amp:card:focus:model"  content="trap">
  <link rel="stylesheet" href="card.css">
</head>
<body>
  <div class="row"><span>Display Name</span><input id="name" class="input"></div>
  <div class="row"><span>Theme</span>
    <select id="theme" class="input">
      <option value="dark">Dark</option><option value="light">Light</option>
    </select>
  </div>
  <button id="save" class="btn" type="button">Save</button>
  <script src="card.js"></script>
</body>
</html>
```

```javascript
// card.js — strict-CSP-safe: no inline handlers, no eval.
const amp = window.amp;

async function load() {
  const profile = await amp.read('users', 'profile', amp.member.ID);
  if (profile) {
    document.getElementById('name').value  = profile.displayName || '';
    document.getElementById('theme').value = profile.theme || 'dark';
  }
}
async function save() {
  const result = await amp.submit({
    intent:  'updateProfile',
    channel: 'users', attr: 'profile', itemID: amp.member.ID,
    values: {
      displayName: document.getElementById('name').value,
      theme:       document.getElementById('theme').value,
    },
  });
  if (result.ok) amp.back();
}
document.getElementById('save').addEventListener('click', save);
load();
```

Inline `<style>` blocks are permitted (the served CSP allows `'unsafe-inline'` for styles); a sibling `card.css` is preferred for anything more than a handful of rules.

### 8.7 Card guidelines

1. **One card per item.** A card is the detail view of one item, not a list-renderer. Lists render natively in the host. (See §8.1.)
2. **Sibling-file bundle.** `index.html` + `card.js` + `card.css` in one directory. No bundler. AI agents emit the whole directory at once.
3. **External script.** Use `<script src="card.js">` — the served CSP forbids inline `<script>`. Inline event handlers (`onclick="…"`) are forbidden for the same reason; bind via `addEventListener`.
4. **Viewport 320x640.** Phone portrait, half-screen. Scales inside any Pane size.
5. **Touch-first.** 44px minimum tap targets. No hover-dependent UI.
6. **CSS variables for theme.** All colors via `--amp-*`; the Pane injects the planet theme.
7. **Always use `window.amp`** for data; never `fetch` directly.
8. **Manifest in `<head>`** so the Pane can introspect title and intents.
9. **`amp.submit(...)`** for forms; `amp.write(...)` for direct CRDT writes; both work, `submit` carries an `intent` the Pane can act on.
10. **Standalone debug.** The fallback `amp` shim logs everything to console for browser-only testing.

---

## 9. Cross-planet addresses

Amp is many planets, not one. A user's home planet, a project's collaboration planet, a public-share planet, a partner organization's planet — each has its own `tag.UID`. An item is addressed across planets by an `amp.Address` — on the wire a single opaque base32 string packing 3–5 UIDs (element / +edit / +planet). (`Address` is the cross-planet addressing token throughout the SDK and wire; it carries the element/planet identity.)

```tsx
// An Address arrives from the server (e.g. on a read or a share); the SDK
// treats it as opaque — use the string as-is.
const addr = addressFromServer;

// Embed `addr` in a share link or a withdraw delegation.  Reading the cited record
// today is a planet-scoped query against its planet (anonymous if that planet is a
// registered public share); the one-call resolve(address) REST primitive lands at
// M5.
```

Cross-planet addressing is the substrate for shareable links: the share URL embeds the packed Address string, and the public viewer's `@art-media-platform/web` resolves it via the public planet without auth. Cross-planet equivalence claims sit on top: a member who exists on multiple planets can publish an Equivalence asserting that two `MemberTag`s refer to the same self.

---

## 10. Whitelabel & Deploy Configuration

Two things get conflated; keep them apart:

- **Your app's own branding is yours.** Your web UI — logo, theme, copy, fonts — lives in your app. amp never dictates your chrome.
- **Per-deploy *host* configuration is operator-side and invisible to your SDK code.** The identity of the planets you display, the web origins allowed to call the host (CORS), the anonymous-readable share planet, deep-link schemes, and the admin allowlist are configured by whoever operates the `ampd` deploy and enforced by the host. **Your client is constructed with exactly `vaultUrl` + `planetTag` (+ an optional share-planet tag) and nothing else** — typically build-time env vars (`VITE_AMP_VAULT_URL`, `VITE_AMP_PLANET_TAG`, `VITE_AMP_PUBLIC_SHARE_PLANET_TAG`). You never ship, fetch, or parse a deploy-config file.

A single `ampd` can host many planets/orgs at once; that multi-tenancy is a host capability, not something your app coordinates.

### 10.1 Deployment vs. Signed Substrate Channels

The durable model: per-deploy configuration is **signed, chronicle-tracked facts on the substrate**, rotated by a signed write — never a static file hand-edited-and-restarted. There is no consumer-facing config file to build against. Three records, by purpose:

**1. A planet's identity — its `Brand` record.** Each public / org planet carries one `Brand` item at a fixed CRDT address (the `amp.brand` attr), admin-signed and edit-chained — so a rebrand (rename, domain change, scheme update, federation roam) is a single signed edit, no restart and no re-genesis. Fields: `AppName`, `AppDomain`, `OrgName`, `OrgHomeURL` / `AppHomeURL`, `URLSchemes`, `Targets[]` (per-platform installs, each an `AppTarget` — incl. `AppleTeamID`), `Links[]`, `CrateSnapshotURL`, `BundledCrates`, `TemplateSet`, and `NamedBy` (the federation that names this planet — the §4.6 back-edge). The personal home planet is "naked" (no `Brand`); its display name is `PlanetEpoch.Label` ("Home").

> **Display-only — the durable security rule.** A planet's substrate `Brand` is read for **display only**: planet header, picker tile, publisher attribution. Because it is admin-mutable, it is **never** read for app *behavior*. Every behavioral field — your deep-link schemes, link host, install targets, crate feed — is read from your **build's own factory brand** (bundled in the app / SKU), never from a planet's `Brand`. So a planet rebrand changes what it *displays as*, never what your app *does*. (One resolver returns `planetBrand ?? factoryBrand` for display; behavioral code reads the factory directly.)

**2. Host gating config — CORS origins + admin allowlist.** The web origins authorized to call the host (the CORS allow-list) and the operator-admin allowlist (who may hit `/api/v1/admin/*`) are **operator HTTP-gating config, not planet governance**. Stable field semantics: a list of allowed origins (`https://maplable.com`, `*.maplable.com` — never pair `"*"` with credentials; see `SECURITY-amp-web-SDK.md`) and a list of admin member-IDs (`eth:0x…`, `email:…`; empty fails closed → every admin call `403`). Stored operator-side, per whitelabel domain — never in your bundle or code path.

**3. The share planet.** Anonymous-readable content lives on a separate public planet (`PlanetEpoch.IsPublic = true`); the operator genesis's it once and registers it host-side (§6.4). Your client points at it via `VITE_AMP_PUBLIC_SHARE_PLANET_TAG`.

**What this means for you:** none of this is in your bundle or code path — construct with `vaultUrl` + `planetTag` (+ share tag). What your operator needs *from you*, once, at deploy time is a short, stable list: the web origins to allow (CORS) and the share-planet name. The field *semantics* are the stable integration surface; where they're stored is operator-side and changes nothing in your integration.

> **Operators:** the host reads CORS, the admin allowlist, and the share-planet UID from operator-side config (`app.brand.json` per whitelabel domain; documented in `deploy/`). It is deliberately **not** part of the consumer contract — don't build against the file. Planet identity and deep-link display are substrate-native `Brand`, resolved from the federation NameService — not this file.

---

## 11. Common patterns

### List with pagination

```tsx
const { data, hasMore, loadMore, loading } = useAmpQuery<ProjectMeta>('projects', 'meta', {
  limit: 20,
  orderBy: '_UpdatedAt',
  filter: { ownerID: member.ID },
});

return (
  <>
    {data.map(p => <ProjectCard key={p._ItemID} project={p} />)}
    {hasMore && <button onClick={loadMore} disabled={loading}>Load More</button>}
  </>
);
```

### Optimistic update

```tsx
const { upsert } = useAmpMutation();
async function rename(itemID, newName) {
  await upsert('projects', 'meta', itemID, { name: newName });
  // useAmpQuery re-renders with the new value as soon as the WS update arrives.
}
```

### File upload with preview

```tsx
const { upload, progress } = useAmpUpload();
const [preview, setPreview] = useState(null);

async function handleFile(file) {
  setPreview(URL.createObjectURL(file));   // instant local preview
  const blobRef = await upload(file, 'projects', { attr: 'media' });
  await upsert('projects', 'media', blobRef.UID, { blobRef, filename: file.name });
  // blobRef.URI is populated by /media/resolve, not by upload — for the durable
  // stream URL use useAmpMedia(blobRef.UID) (or client.resolveMedia(blobRef)).
}
```

### Anonymous public-share read

```tsx
// Constructed without a session — the share planet is open-read.
const shareClient = new AmpWebClient({ vaultUrl, planetTag: import.meta.env.VITE_AMP_PUBLIC_SHARE_PLANET_TAG });
const { data } = await shareClient.query<{ snapshotChannel: string; snapshotAttr: string; snapshotItemID: string }>('shares', 'link', { itemID: slug });
const link = data[0];
// Follow the pointer with a planet-scoped read on the same public planet:
const { data: snap } = await shareClient.query(link.snapshotChannel, link.snapshotAttr, { itemID: link.snapshotItemID });
```

### Cross-device subscribe for widget instance

```tsx
const { data } = useAmpQuery('widgets', `instance.${member.ID}`, {});
// fires on every cross-device edit by this member; per-member partition makes the
// subscribe naturally scoped without a filter predicate.
```

---

## 12. What NOT to do

1. **Never bypass `@art-media-platform/web`** to call `/api/v1/*` directly from app code. The client wraps the wire shape with retry, session refresh, and sealed-box helpers; raw fetches will accumulate bugs.
2. **Never store user-supplied secrets unsealed** in channel items. Run `seal()` before `upsert`. Plaintext API keys in `users/api_keys_overrides` is the single most common security mistake.
3. **Never assume immediate consistency.** Writes propagate over WebSocket; design UI optimistically.
4. **Never push SecurityEvent telemetry** to a replicated channel. Audit logs and rate-limit notifications are local-only.
5. **Never use `window.fetch`** inside a card. Always go through `window.amp.*`.
6. **Never gate UX on `member.kind`** at the protocol layer, and **never encode a payment/subscription tier in it**. `Kind` is an identity taxonomy (Person / Group / Agent / Memorial), not an entitlement. Apps may surface Kind in UI; model billing tier as app data + per-channel ACC (§14.4).
7. **Never name specific crypto algorithms** in code or docs. Use `seal/open`, `sign/verify`, `hash`, `safe.KeyRef`.
8. **Never build on bulk namespace enumeration.** `/api/v1/search` is membership-gated, best-effort discovery over the federations you've joined — not a public directory dump, and never anonymous. Resolve exact FQDNs you already know; don't crawl. (§4.6)

---

## 13. Glossary

| Term | Meaning |
|---|---|
| **Planet** | A self-governing community with its own members, channels, and encryption keys. |
| **Channel** | A named CRDT container within a planet; addressed by `tag.UID`. |
| **Attribute (attr)** | A typed data field within a channel; addressed by `tag.UID`. |
| **Item** | A single CRDT record, identified by `tag.UID`. |
| **Edit** | A versioned update to an item; CRDT-merged. |
| **TxMsg** | A signed, encrypted transaction containing one or more data ops. |
| **Verb-RPC** | A `POST /api/v1/tx` carrying `InvokeURL: amp://~/{app}/{verb}` — routes the batch to an app's verb handler (`PinMode_Invoke`, not journaled) which authors the durable write custodially; the write path for `Access_ReadOnly` channels. SDK `invoke()` (§4.3). |
| **Epoch** | A key rotation period; new epoch = new encryption key for the planet. |
| **BlobRef** | A reference to a binary blob stored outside the TxMsg. |
| **Vault** | An `ampd` peer that stores and relays encrypted data; cannot read content. |
| **Portal** | `app.www` — the unified HTTP service inside `ampd` (`/www/*` asset streaming, REST, WebSocket, deep links, static sites, cards). |
| **Member** | An authenticated identity within a planet. Has `kind`: Person / Group / Agent / Memorial / etc. |
| **MemberProof** | An HMAC of derived proof key over TxID; lets a relay verify membership without decryption. |
| **WebRect** | A Unity 3D Pane that renders a web card. |
| **Pane** | The host (Unity WebView panel or 2D drawer) of a card. |
| **Panel** | Identical to Pane in principle, though it adheres to different UI handling. |
| **Card** | A self-contained HTML document rendered as the detail view of a single item, opened on activation. Speaks to amp via `window.amp`. |
| **Card manifest** | `<meta name="amp:card:*">` tags in a card's `<head>` declaring title, intents, focus model. |
| **Address** | The cross-planet addressing token — on the wire a single base32 string packing 3–5 UIDs (element / +edit / +planet). The SDK treats it as opaque (use the base32 string as-is). Carries the element/planet identity. |
| **Equivalence** | A symmetric claim that two addresses refer to the same thing in a stated context. |
| **Withdraw** | A signed signal that the signer no longer consents to a cited record. Carries `subject` (whose consent) + optional `delegation` (a packed Address citing the record that grants authority) when a delegate speaks for someone else. |
| **Share planet** | A planet operating in `PlanetEpoch.IsPublic = true` mode — anonymous-readable, member-writable. The operator performs planet genesis (`amp planet create --tag <name>` / `POST /api/v1/admin/planet/create`) and registered host-side; see §6.4 / §10. |
| **Admin endpoint** | Bearer-authenticated server endpoint reserved for operator-driven substrate operations — currently `POST /api/v1/admin/planet/create` for share-planet genesis. |
| **ChannelEpoch** | A channel's per-epoch ACC + access grants. Committed via `POST /api/v1/governance/grant` (§14.4). |
| **NameService** | amp's federation directory — resolves a registered FQDN to the planet that serves it and where its vault is dialable. `resolve` is anonymous; `search` / `federation/peers` are Bearer-gated. See §4.6. |
| **FQDN** | A fully-qualified domain name (`spaces.example.com`) registered in a federation's NameService and resolvable to a planet. |
| **Federation** | A set of planets whose names a session resolves over once it has joined; the unit of namespace reachability. |
| **TrustState** | A resolve verdict — `Verified` / `Refuted` / `Unchecked` — from checking the planet's `Brand` back-edge against the answering federation. The UI must not silently pick when it isn't `Verified`. |

---

## Wire-Contract Drift Guard

The JSON wire shapes in this SDK are hand-mirrored between Go
(`amp/webapi/webapi.types.go`) and TypeScript (`src/types.ts`).  Shared golden
fixtures pin the contract so drift on either side fails tests instead of
shipping:

- **Fixtures** — `amp/webapi/testdata/*.json`: login (all credential schemes),
  tx/item (`_ItemID`/`_EditID`/`_FromID` metadata keys), subscribe frames,
  edit chains, the full invite family (issue/accept/revoke/list with ledger
  rank), vault endpoints (base64 `Address` — opaque bytes, not a UID),
  media/Tag shapes (`ContentTypeRaw`), enum-name goldens (AccessLevel /
  WithdrawReason / InviteStatus / TrustState), and the CryptoKitID small-int ↔
  `tag.UID` mapping.
- **Go side** — `go test ./amp/webapi/` decodes every fixture with
  `DisallowUnknownFields` and asserts a lossless re-marshal.
- **TS side** — `npx vitest run src/drift.test.ts` checks every fixture
  against hand-listed per-interface key tables (the tables ARE the drift
  guard) plus the enum and kit-mapping goldens.

When a shape changes intentionally, update the fixture and both tables in the
same change — the fixture is the contract; Go is the reference implementation.

## 14. Deploying amp: topologies, identity & versioning

This section answers the architecture questions that come up when an existing product — with its own auth, its own payments, its own offline story — folds amp in. It describes the **current** model; where something isn't built yet, it says so and gives the bridge.

### 14.1 Deployment topologies — where the vault runs

**The web SDK is a pure client — it never builds, embeds, or runs a vault.** For a web app the vault is an `ampd` host an operator already runs, reached over HTTPS (§0). The shapes below describe where *that host* lives — an operator / deploy concern, not something your web build ships.

An `ampd` host is a full peer: it stores, signs, encrypts, and relays.

- **Shared cloud vault (the default for a web app).** One `ampd` on a server is the rendezvous point for cross-device sync and multi-member collab; your client connects to it over HTTPS (for Maplable, `https://prod.plan.tools`). Simplest to operate; the app needs the server reachable to read / write.
- **Embedded local vault (ADVANCED — bundled desktop/Electron only; *not* a web build).** Bundle `ampd` (or the `libampd` shared library) inside a desktop / Electron app and spawn it as a child process / link it in-process. The user *is* the vault; reads, writes, and seals work with no network — this is what preserves an **offline-first** desktop product. It is the one topology that ships the Go host (cross-compile per target — darwin-arm64/x64, win-x64/arm64, linux-x64 — and code-sign each), and it is **not** part of a web-SDK integration. Reach for it only if you are explicitly building a bundled native app.
- **Hybrid.** An embedded local vault for offline work plus a cloud vault as a sync / relay peer. CRDT writes queue locally and replicate when the cloud peer is reachable. The native amp model for a desktop app, not a bolt-on.
- **Self-hosted & federated (the licensee path).** A partner runs their *own* `ampd` and is therefore their own operator — their own CORS, planets, and admin, with no per-deploy config handoff to you. They join the wider network by **federating** with a parent deploy, so their planets are discoverable through it (and vice-versa). See §14.8.

A pure-JS / WASM in-process vault is **not** on the near-term path; the embeddable unit today is the native host (`ampd` / `libampd`). For a paid **desktop** app, **hybrid (embedded + optional cloud sync)** is the recommended shape — but a **web** app needs none of this: it connects to an operated node.

### 14.2 Offline membership & feature gating

Be precise about "offline," because it splits in two:

- **Data is offline-first by construction.** Against a local / embedded vault, reads, writes, and `seal`/`open` all work with no network; writes queue and replicate later.
- **The session token is online-issued.** `login()` returns an opaque random `SessionToken` (a Bearer) minted by the host. It is **not** a self-verifiable signed capability, so a *cloud-only* deployment cannot confirm membership while the network is down.

The way to get the equivalent of a locally-verifiable signed license token is the **embedded local vault** (§14.1): the member's signed membership lives in the local host and verifies locally — no phone-home. The *portable* signed capability is the `memberToken` login scheme (`signed(memberID‖ts)`, verified against the member's seated signing key — §4.1) — **available on a host-bridged node**: a co-located client (e.g. a Unity WebView on loopback) has the local `ampd` mint it from the held key, so an embedded deployment issues a verifiable session without phone-home. It returns `501` only on the in-memory dev backend (§14.7).

For a cloud-only build, **degrade gracefully**: when the host is unreachable and entitlement can't be confirmed, disable the gated action cleanly (grey out "Share") rather than letting it fail mid-flight.

### 14.3 Device-scoped / anonymous identity

Two facts that are easy to conflate:

- **Sealing does not require a human identity.** The device-local `EncryptKey` (§6.2) is generated and persisted independently — `seal`/`open` are a property of *the device*, not of an email / wallet / DID.
- **Persisting to a planet requires a session.** Writing a channel item — even a sealed BYOK blob — is a signed TxMsg against a planet, which needs a logged-in member.

There is **no first-class "issue an anonymous device member" API** today — don't wait on one. To ship BYOK before unifying human identity: keep gating app launch however you do now, log the member into their auto-provisioned home planet with whatever credential you already have, and store sealed secrets under a device-stable item ID. Sealing protects the secret from the host and other members regardless; the login is just what gives you a planet to write to.

### 14.4 Membership tiers, Stripe, and the admin surface

- **Don't put the tier in `member.kind`.** `Kind` is an identity taxonomy, not an entitlement, and the protocol never gates on it (§12). Model **payment tier as application data** your app reads (e.g. a `members/billing/{memberID}` item) and enforce capability with **per-channel ACC** — gate `projects.share`, `users.api_keys_overrides`, etc. on the member's access grants.
- **Admin surface that exists today:** `POST /api/v1/admin/credentials/email/issue` (Bearer + the operator admin allowlist — operator-side gating config, §10) mints an email-scheme member and returns `{ MemberID, Email }`. This is what a Stripe webhook calls server-side to provision a paying customer. Sealed invites ride the same membership surface: `POST /api/v1/invite/issue` mints a single- or multi-use sealed invite (with `/revoke` + `/list`) and `POST /api/v1/invite/accept` redeems it (accept is the client-side join — §4.7; both Bearer, both need the production `SessionBackend` and return `501` on the in-memory dev backend).
- **The tier → ACC grant surface is wired:** `POST /api/v1/governance/grant` (Bearer) commits a channel's complete `ChannelEpoch` — `MemberGrants` + `DefaultGrants`, plus an optional `Parent` channel and cited attestations. Semantics are **latest-wins-REPLACE**: to change one member's grant, read the current epoch, modify it, and re-commit the whole set (read-modify-write). This is where you enforce the tier you modeled as app data (above) — gate `projects.share`, `users.api_keys_overrides`, etc. on these grants, never on `Kind`.

### 14.5 SDK versioning & stability

The SDK is **beta (pre-v400)**. Between minor revs, breaking changes are possible, and there is no public npm channel during beta — you receive a versioned bundle (`amp-web-SDK-vNNN[.P].zip`). One source of truth ties it together: the bundle label, the `package.json` version, and the `amp.SDK` git tag are the same revision. **Pin the bundle you integrated** (vendor it / commit the version) rather than floating; upgrade deliberately and re-run your contract tests. At v400 ("Production Ready") it flips to public GitHub Releases under an open license with a semver compatibility promise.

### 14.6 amp as a signaling / coordination plane

amp pub/sub can replace a bespoke signaling server for collab coordination. Pattern:

- Partition by session into a per-room attr — e.g. `webrtc.signaling.{spaceId}`.
- Each peer `upsert`s its offer / answer / ICE candidate as an item; the other peer subscribes and consumes.
- Membership of the room = who can read / write that attr.

Item order is `_ItemID` (tag.UID) byte order, not wall-clock — fine for SDP exchange; design ICE-candidate handling to tolerate reordering. Keep this to the **coordination / checkpoint plane** (§5.4): the media stream itself stays on WebRTC + TURN. No canonical reference implementation ships yet — this is the recommended shape, not a drop-in.

### 14.7 Integration fixtures

For end-to-end tests without a production vault, point the client at a local `ampd` running its in-memory dev backend — it round-trips the exact wire shape (not encrypted, not synced: a contract fixture, not a vault). The bundle also ships `scripts/smoke.mjs`, the login → upsert → query → seal / open smoke check the SDK validates against. Drive your Stripe **test-mode** webhook at a local fixture vault to exercise provision / expire paths without touching real customers.

### 14.8 Running your own vault & federating (the licensee path)

> **Accepting an invite to *join* a federation does not require running your own vault — that's a web-client call (§4.7).** This section is the heavier, self-hosting path: running your **own** `ampd` so your planets are independently hosted and discoverable. A web app on an operated node needs none of it.

When a partner runs their **own** `ampd` (the §14.1 cloud or embedded topology) they are their own operator — their own CORS, planets, and admin — so there is **no per-deploy config handoff**. What federating with a parent buys them is *discoverability*: their planets resolve through the parent's federation, and the parent's through theirs. A parent federation is itself founded operator-custodially from the CLI — `amp federation found "<label>" --invite-pass … --invite-max … --fqdn <fqdn>` seals the genesis with the operator's own key and emits the multi-use onboarding invite in one act. Two ways to federate with one, both proven over the public member-signed APIs (no special grant — **federation membership is the authorization to publish names**):

- **Join the parent's federation (invited member).** The parent issues an invite (`amp invite issue`); the partner accepts (`amp invite accept`) and is now a member, then registers their own records into the parent's federation: `amp --planet <parent-fed> name register <fqdn> --target <planet> --federation <parent-fed> --vault tcp:their-host:port` (the global `--planet` binds the session to the federation — required from a fresh CLI session). Their names resolve to their planets, carrying their own vault as the bootstrap address.
- **Peer two federations.** The partner runs their *own* federation; the parent links it in with `amp federation peer <partner-fed-UID> --vault tcp:their-host:port`. A resolver then forwards across the directory hop (`ResolveVia`) into the partner's federation. Either side can peer the other.

**Following a federation at boot.** A self-hosted node pins the federations it follows with `ampd -federation <UID>@tcp:host:port` — it dials the bootstrap peer and watches that federation's NameService channel at startup, so its names are carried without a manual resolve-and-pin each boot (connectivity otherwise rides `-vault.peers`).

**Identity & trust.** A partner's planets set `Brand.NamedBy` to the naming federation — the back-edge (§4.6) that lets a resolver return `TrustState: 'Verified'`. Resolved records carry the partner's own `VaultAddrs`, so a consumer reaches the partner's vault **directly** — cross-host, never proxied through the parent.

The CORS boundary from §10 still holds: on a *shared* host the operator allow-lists web origins; a self-hosted partner sets their own. Operator runbook with the exact genesis / invite / register / peer steps: `deploy/README`.

---

*This SKILL is maintained by the art.media.platform engineering team. For the protocol primer and primitives, see [`amp.SDK`](https://github.com/art-media-platform/amp.SDK); for the security model, see the companion `SECURITY-amp-web-SDK.md` in this bundle.*
