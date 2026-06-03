# SKILL: amp-web-SDK

> **What this document is:** Instructions for an AI coding agent (Claude Code, Cursor, Replit Agent, etc.) — and a contract for human web developers — to build web applications that persist data through the art.media.platform (amp) vault infrastructure. Drop this file into any web project. Generated code uses the `@art-media-platform/web` library to talk to `app.www`, the unified HTTP service inside `ampd`.

> **What amp is (30-second version):** amp is a decentralized storage and communication protocol where data is encrypted, signed, and replicated across independent nodes ("vaults"). Vaults relay traffic without reading content. Every host is a full peer — your app works offline; sync is additive. Your web app is a disposable UI layer; amp is the durable substrate underneath.

---

## 1. Scope of This Contract

This SKILL is the contract a web app codes against. It has one consumer and one server:

- **Server:** `app.www`, an amp app inside `ampd`, owning HTTP port 5193. The wire shape (REST verbs, WebSocket event format, auth model) is the contract.
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

**AI Agent rules.** When generating or modifying code for a web project that uses amp:

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

Publishing to npm as `@art-media-platform/web` lands at v400.

### Provider Configuration

```tsx
import { AmpProvider, AmpWebClient } from '@art-media-platform/web';

const client = new AmpWebClient({
  vaultUrl: import.meta.env.VITE_AMP_VAULT_URL,    // e.g. https://my-amp-node:5193
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
VITE_AMP_VAULT_URL=https://my-amp-node:5193
VITE_AMP_PLANET_TAG=my-planet-tag

# When reading anonymous shares, point at the share planet:
VITE_AMP_PUBLIC_SHARE_PLANET_TAG=my-planet-tag-shares
```

**Server-side prerequisite for share planets.** `VITE_AMP_PUBLIC_SHARE_PLANET_TAG` only resolves anonymously when the deploy operator has actually registered the planet on the server side. Two mechanisms (see §6.4):

- **Persisted via `app.brand.json`** — the deploy's brand config carries a `SharePlanet { Name, UID }` block; the portal registers it with the bridge at boot and on each snapshot reload.
- **Runtime via `amp planet create`** — the operator hits `POST /api/v1/admin/planet/create` (or runs the CLI) once, drops the returned UID into `app.brand.json` for persistence.

Without one of these, anonymous reads against the planet tag fall through to the standard Bearer gate.

---

## 4. Wire Shape — the Contract

`app.www` listens on port 5193. Every endpoint accepts `Authorization: Bearer <session-token>`, except where noted. CORS is per-org and configured in the deploy's `app.brand.json`. The server emits `Content-Type: application/json; charset=utf-8` on JSON responses.

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

The unified `/api/v1/login` is **shipped**: `wallet`, `email`, and `did` are fully wired and Bearer-issuing; `memberToken` and `yubikey` parse cleanly and return HTTP 501 with `Code: "Unsupported"` until they land — SDK clients can lock the contract today, and the remaining schemes flip on without any wire-shape change.  Non-2xx responses throw a typed `AmpError` carrying the wire `Code` (surfaced as `AmpError.code`, e.g. `AmpErrorCode.Unsupported`) plus the HTTP `status`, so a client can dispatch on the code and treat a not-yet-wired scheme as a no-op. The cookie-bound legacy path at `/api/v1/login/wallet/{challenge,verify,session,logout}` remains for browser flows that prefer it; both paths share one session store.

**DID scheme (W3C DID 1.0 — login only).** `did` proves control of the key a DID URI names: fetch a challenge with `?did=<uri>`, sign it, and submit `{ Scheme: 'did', DID, Signature, Nonce }`.  Shipped methods: **`did:key`** (Ed25519) and **`did:pkh:eip155`** (Ethereum wallet).  A `did:pkh:eip155:*:0x…` login folds to the *same* MemberID as a `wallet` login over that address (`eth:lc(addr)`) — two URI spellings of one key, one member.  A DID whose method/curve isn't wired yet (e.g. `did:key` P-256/secp256k1, `did:pkh:solana`, `did:web`) returns the same 501 `Unsupported`.  This is DID-Auth — Verifiable Credentials (issuer-signed claims) are out of scope.

**Email scheme additionally exposes recovery + admin-issue endpoints:**

```
POST /api/v1/login/email/recover     (anonymous; returns 202 uniformly)
POST /api/v1/login/email/redeem      (anonymous; consumes token, mints session)
POST /api/v1/admin/credentials/email/issue   (Bearer; admin-gated signup)
```

MemberID for the email scheme = `tag.NameFrom("email:lc(addr)").ID` — mirror of the wallet path's `tag.NameFrom("eth:lc(addr)").ID` rule.  Failure modes (unknown email, wrong password) return a single 401 envelope with timing-uniform KDF cost so the response carries no existence oracle.  Bulk email (campaigns, white-label outreach) routes through `app.email`'s queue channel; see PRD-app-email for the campaign-side contract.

```typescript
interface AmpMember {
  ID: string;                  // member tag.UID, base32
  DisplayName: string;
  Email?: string;
  PlanetID: string;            // planet tag.UID, base32
  Kind?: string;               // tag.UID resolving to a LawMemberKind_* (DESIGN-11). Default: Person.
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
       Body: { Ops: TxOp[], PlanetTag?: string }
       Response: { TxID, Results: Array<{ ItemID, EditID }> }

TxOp =                                                          // Kind values stay lowercase
  | { Kind: 'create',   Channel, Attr, Value, ItemID? }
  | { Kind: 'upsert',   Channel, Attr, ItemID, Value }
  | { Kind: 'remove',   Channel, Attr, ItemID }
  | { Kind: 'withdraw', Channel, Attr, ItemID, Withdraw: WithdrawNote }   // see §7 + DESIGN-15

POST   /api/v1/channels/{channel}/attrs/{attr}/items                       ─ sugar: tx with one create op
PUT    /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}              ─ sugar: tx with one upsert op
DELETE /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}              ─ sugar: tx with one remove op
POST   /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}/withdraw     ─ sugar: tx with one withdraw op
```

`planetTag` on the canonical `/api/v1/tx` endpoint, and the `?planetTag=<...>` query param on the sugar verbs, target a planet other than the session default — the same way the read endpoints work.

**Atomicity contract.** All ops in a single `POST /api/v1/tx` ride one TxMsg under one encryption context. Batches that span encryption domains (a planet-public op alongside a private-channel op, or two different private channels) are rejected; split into separate `tx` calls.

**WebSocket fan-out.** A 10-op tx produces 10 `update`/`delete`/`withdraw` WebSocket events on the consumer side — one per op. Subscribers receive item events; they don't need to know about transaction boundaries.

**Tombstone semantics.** `remove` writes a tombstone, not a wipe; bytes survive in the journal bound by retention. `withdraw` is the parallel signed signal — see §7.

### 4.4 Media Upload

```
POST   /api/v1/upload
       Content-Type: multipart/form-data
       Fields: file (required), channel, attr, planetTag (optional)
       Response: amp.Tag (UID + URI + ContentType + I/Units=Bytes)

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
  ContentType?: string;    // MIME type
  I?: number;              // plaintext byte length (when Units = Bytes)
  Units?: number;
}
```

**Caller-carries-the-Tag.** The cabinet (channel item that surfaced the BlobRef) is the source of truth for blob metadata.  When you need to render a blob in `<img>`/`<video>`, send the blob's `amp.Tag` (read from the cabinet) to `POST /api/v1/media/resolve`; the host's asset publisher maps it to a streamable `/www/{UID}` URL.  The publisher is in-memory and idempotent — repeated resolves dedupe, vault outage / restart / cross-vault read all just republish on demand.  No DESIGN-9 cold-store window for filenames or ContentType; no persistent publisher state to migrate.

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
```

`FromID` is always the TxMsg signer (the member who authored the op). On a `withdraw` frame, `Withdraw.Subject` names whose consent is being withdrawn — equal to `FromID` in the common case (signer is the subject), distinct when an authorized delegate (Memorial, GDPR delegation per DESIGN-14) speaks on the subject's behalf. `Withdraw.Delegation` is a base32-packed `amp.Address` citing the record proving that authority. `Subject`/`WithdrawnBy` are plain base32 UID strings. See §7 + DESIGN-15.

Subscriptions are per-`(channel, attr)` and deliver every item event on that attr. To scope subscribe by item, partition the data into per-scope attrs at write time (e.g., `widgets/instance.{memberID}` rather than `widgets/instance` filtered by ownerID).

The server NEVER pushes telemetry frames (failed-login, rate-limit, audit) over the consumer WebSocket. SecurityEvent telemetry is local-only by design.

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

For `did:pkh:eip155` the signer is the same EVM-wallet `personal_sign` as above (the DID just carries the address), and the resulting member is identical to the `wallet` path. For `did:key` the signer is whatever holds the Ed25519 private key (client-custody is an open UX question — see PRD-did-identity §9).

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
const { tx, create, upsert, remove, withdraw, loading } = useAmpMutation();

// One TxMsg, many ops — atomic, single signature, single MemberProof:
const results = await tx([
  { Kind: 'upsert', Channel: 'projects', Attr: 'labels',   ItemID: l1, Value: lv1 },
  { Kind: 'upsert', Channel: 'projects', Attr: 'labels',   ItemID: l2, Value: lv2 },
  { Kind: 'upsert', Channel: 'projects', Attr: 'articles', ItemID: a1, Value: av1 },
  { Kind: 'remove', Channel: 'projects', Attr: 'circles',  ItemID: c1 },
]);

// Single-op convenience wrappers (each is one one-op tx under the hood):
const itemID = await create('projects', 'snapshot', value);
await upsert('users', 'profile', member.ID, patch);
await remove('projects', 'snapshot', itemID);
await withdraw('shares', 'link', itemID, {
  reason: 'Departed',
  rationale: 'left the team',
  // subject + delegation are optional — omit when the signer is the subject.
  // (WithdrawOpts keys are camelCase — an SDK option bag, never serialized.)
  subject: deceasedMember.ID,                            // DESIGN-15 delegated path
  delegation: delegationAddress,                         // base32-packed amp.Address
});
```

`tx(ops)` is the canonical write — all ops in one batch ride a single TxMsg, sealed under a single encryption context, with one signature and one MemberProof. A debounced project save with 50 entity changes is **one** TxMsg, not 50. Mixing encryption domains in one batch (planet-public alongside private-channel ops) is rejected — split into separate `tx()` calls.

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
// client.address() is an identity passthrough kept for symmetry.
const addr = client.address(addressFromServer);   // base32 string in, string out

// Reading a planet you can reach (a public share, or a cross-planet record) is a
// planet-scoped query today — pass planetTag to query.  The one-call
// resolve(address) REST primitive lands at M5 (PRD-app-www §8):
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

**Already holding a UID?** A 26-char base32 `tag.UID` (e.g. `member.ID`, or an `itemID` from a read) is already in wire form — pass it straight through. `resolveTag` round-trips a UID back to the same UID rather than re-hashing it, so mixing names and UIDs in one `resolveTags` batch is safe.

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

The deploy operator creates the share planet and registers it with the running `ampd` daemon. Two paths:

**CLI:**
```bash
# After wallet-login via the web (POST /api/v1/login scheme=wallet),
# pass the resulting Bearer token to the CLI:
AMP_TOKEN=<bearer> amp planet create --tag <name>
# → { "PlanetID": "<base32 UID>", "Tag": "<canonic>", "Public": true }
```

**Brand JSON (persistence across daemon restarts):** drop the resulting UID into `app.brand.json`'s `SharePlanet` block alongside the same canonic name:
```json
"SharePlanet": {
  "Name": "Maplable-shares",
  "UID":  "<26-char base32 UID>"
}
```

The portal validates `tag.Parse(Name).ID == UID` at boot — refuses to start with a mismatched pair so silent drift between brand JSON and the planet-genesis ceremony surfaces immediately.

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
- `Kind?` — `tag.UID` resolving to a `LawMemberKind_*` definition (DESIGN-11). Default `LawMemberKind_Person`. Apps surface this in UI but do NOT gate behavior on it; communities decide what each Kind can do via per-channel ACC, not via protocol-level rules.
- `Address?` — 0x-prefixed wallet address; present for wallet-scheme members.

A given human can have many `MemberTag`s — one per planet they belong to. Cross-planet equivalence is a DESIGN-14 Equivalence claim, not an automatic merge.

---

## 7. Soft delete & withdrawal

The chronicle is immutable. `DELETE /api/v1/channels/{channel}/attrs/{attr}/items/{itemID}` writes a tombstone — a TxOp that the resolved-state view interprets as "no longer authoritative." Bytes survive in the journal bound by retention.

For records about a member that the member wants disowned (a project shared on a public planet, a profile attribution), the right primitive is **DESIGN-15 Withdraw** — a parallel signed signal. `withdraw('shares', 'link', itemID, { reason, rationale })` writes a Withdraw record. The original item stays signed and visible; consumers see both the original and the withdrawal, and choose honoring policy (suppress, redact, recontextualize). Withdraw reasons (the `WithdrawReason` union — the wire value is the bare enum name) are `Consent`, `Inaccuracy`, `Outdated`, `Coerced`, `Forgotten`, `Departed`, `InviteRecall`, `Retracted` (plus `NoReason` = zero sentinel).

### 7.1 Wire surface

A `withdraw` op carries the full DESIGN-15 information set:

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

`Subject == WithdrawnBy` is the common case — the signer is the subject. They differ only when an authorized delegate (Memorial speaking for a deceased member, GDPR delegation per DESIGN-14, etc.) issues the withdrawal on the subject's behalf; then `delegation` cites the record that grants that authority.

### 7.2 UX distinctions

| Verb | Maps to | Used for |
|---|---|---|
| Delete | `remove` | "Remove this from my project list" — owner removes their own record |
| Unshare | `withdraw` | "Take down the public copy" — record stays signed; consumers see the withdrawal |
| Forget me | `withdraw` with `Reason=Forgotten` | "Right-to-be-forgotten" request; per-jurisdiction node operators may also delete bytes locally to comply with law |
| Memorial unshare | `withdraw` with `subject` + `delegation` | An authorized delegate withdraws on behalf of someone who can no longer act for themselves (DESIGN-15 §3) |

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
  subscribe(channel: string, attr: string, cb: (event: SubscribeEvent) => void): () => void;

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

Cards reach the bridge via `window.amp` injected by the host (Unity WebView, browser shim, or test harness). A standalone-browser fallback logs every call to console for debugging:

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

Amp is many planets, not one. A user's home planet, a project's collaboration planet, a public-share planet, a partner organization's planet — each has its own `tag.UID`. An item is addressed across planets by an `amp.Address` — on the wire a single opaque base32 string packing 3–5 UIDs (element / +edit / +planet). (`Address` is the cross-planet addressing token throughout the SDK and wire; it carries the DESIGN-12 element/planet identity.)

```tsx
// An Address arrives from the server (e.g. on a read or a share); the SDK
// treats it as opaque and passes it through.  client.address() is an identity
// passthrough kept for symmetry.
const addr = client.address(addressFromServer);

// Embed `addr` in a share link or a withdraw delegation.  Reading the cited record
// today is a planet-scoped query against its planet (anonymous if that planet is a
// registered public share); the one-call resolve(address) REST primitive lands at
// M5 — PRD-app-www §8.
```

Cross-planet addressing is the substrate for shareable links: the share URL embeds the packed Address string, and the public viewer's `@art-media-platform/web` resolves it via the public planet without auth. Cross-planet equivalence claims (DESIGN-14) sit on top: a member who exists on multiple planets can publish an Equivalence asserting that two `MemberTag`s refer to the same self.

---

## 10. Whitelabel & per-org configuration

`app.www` matches the inbound `Host` header against per-org configs in `{persistPath}/orgs/{domain}/`. Each org carries:

- `app.brand.json` — identity, deep-link schemes, per-platform install targets, allowed CORS origins, planet tag bindings (main + share).
- `www/` — static-site root. Marketing pages, the SPA, service worker, etc.
- `apple-app-site-association` / `assetlinks.json` — universal/app-link configs.
- `open-link.tmpl.html` — deep-link landing page.

A single `ampd` instance can serve many orgs by `Host`-header routing — each with its own `www/` SPA, planet bindings, and admin list. There is no hard per-instance cap on orgs or planets; it is bounded by host resources.

`app.brand.json` is **server-side operator config**, not something the SDK fetches. The client is handed the planet tag(s) it needs at construction time (`vaultUrl` + `planetTag`, plus an optional share-planet tag) — typically surfaced into the SPA as build-time env vars (`VITE_AMP_VAULT_URL`, `VITE_AMP_PLANET_TAG`, `VITE_AMP_PUBLIC_SHARE_PLANET_TAG`). The brand file's job is to configure the *host*: CORS, deep links, the admin allowlist, and which planet is anonymous-readable.

### 10.1 `app.brand.json` shape

```json
{
  "AppName":        "Maplable",
  "AppDomain":      "maplable.com",
  "AppDesc":        "Offline-first 3D mapping",
  "OrgName":        "Maplable Inc.",
  "AllowedOrigins": ["https://maplable.com", "*.maplable.com"],
  "URLSchemes":     ["amp://", "maplable://"],
  "Targets":        [ { "Platform": "iOS",     "DownloadURL": "..." },
                      { "Platform": "Android", "DownloadURL": "..." } ],
  "Admins":         ["eth:0xabc…", "email:ops@maplable.com"],
  "SharePlanet": {
    "Name": "Maplable-shares",
    "UID":  "<26-char base32 UID>"
  }
}
```

| Field | Meaning |
|---|---|
| `AppName` / `AppDomain` / `AppDesc` / `OrgName` | Org identity — surfaced on the deep-link landing page and host-rendered HTML. |
| `AllowedOrigins` | CORS allow-list. List specific origins; never pair `"*"` with credentials (see `SECURITY-amp-web-SDK.md`). |
| `URLSchemes` | Deep-link / universal-link schemes the host advertises. |
| `Targets[]` | Per-platform install targets — `{ Platform, DownloadURL }`. |
| `Admins[]` | Admin allowlist — MemberID expressions (canonic identity strings like `eth:0x…` / `email:…`, or base32 UIDs). Gates the `/api/v1/admin/*` endpoints (§14.4). **Empty or missing fails closed** — every admin call returns `403`. |
| `SharePlanet { Name, UID }` | Optional anonymous-readable share planet (§6.4). |

`SharePlanet` is optional — orgs without a share planet get the standard Bearer-only posture. When present, both `Name` (canonic tag.Name) and `UID` (base32 PlanetEpoch UID) are required and must agree (`tag.Parse(Name).ID == UID`); the portal refuses to boot a mismatched pair so a deploy can't accidentally point at the wrong planet.

**Field set vs storage location.** Treat the fields above as the stable integration surface. Where they *live* is migrating substrate-native: anonymous-readability and the admin allowlist are becoming signed, chronicle-tracked facts on the planet's own `Brand` record (so rotation is a signed write, not an edit-and-restart). The static `app.brand.json` is the current operator surface during that transition; the field semantics carry over.

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
  setPreview(URL.createObjectURL(file));
  const blobRef = await upload(file, 'projects', { attr: 'media' });
  setPreview(blobRef.URI);
  await upsert('projects', 'media', blobRef.UID, { blobRef, filename: file.name });
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
6. **Never gate UX on `member.kind`** at the protocol layer, and **never encode a payment/subscription tier in it**. `Kind` is an identity taxonomy (Person / Group / Agent / Memorial, DESIGN-11), not an entitlement. Apps may surface Kind in UI; model billing tier as app data + per-channel ACC (§14.4).
7. **Never name specific crypto algorithms** in code or docs. Use `seal/open`, `sign/verify`, `hash`, `safe.KeyRef`.

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
| **Epoch** | A key rotation period; new epoch = new encryption key for the planet. |
| **BlobRef** | A reference to a binary blob stored outside the TxMsg. |
| **Vault** | An `ampd` peer that stores and relays encrypted data; cannot read content. |
| **Portal** | `app.www` — the unified HTTP service inside `ampd` (`/www/*` asset streaming, REST, WebSocket, deep links, static sites, cards). |
| **Member** | An authenticated identity within a planet. Has `kind` (DESIGN-11): Person / Group / Agent / Memorial / etc. |
| **MemberProof** | An HMAC of derived proof key over TxID; lets a relay verify membership without decryption. |
| **WebRect** | A Unity 3D Pane that renders a web card. |
| **Pane** | The host (Unity WebView panel or 2D drawer) of a card. |
| **Panel** | Identical to Pane in principle, though it adheres to different UI handling.
| **Card** | A self-contained HTML document rendered as the detail view of a single item, opened on activation. Speaks to amp via `window.amp`. |
| **Card manifest** | `<meta name="amp:card:*">` tags in a card's `<head>` declaring title, intents, focus model. |
| **Address** | The cross-planet addressing token — on the wire a single base32 string packing 3–5 UIDs (element / +edit / +planet). The SDK treats it as opaque; `client.address()` is an identity passthrough. Carries the DESIGN-12 element/planet identity. |
| **Equivalence** | A symmetric claim that two addresses refer to the same thing in a stated context. DESIGN-14. |
| **Withdraw** | A signed signal that the signer no longer consents to a cited record. DESIGN-15. Carries `subject` (whose consent) + optional `delegation` (a packed Address citing the record that grants authority) when a delegate speaks for someone else. |
| **Share planet** | A planet operating in `PlanetEpoch.IsPublic = true` mode — anonymous-readable, member-writable. Configured per-org via `app.brand.json`'s `SharePlanet { Name, UID }` block (boot-time registration) or created at runtime via `amp planet create --tag <name>` / `POST /api/v1/admin/planet/create`. |
| **Admin endpoint** | Bearer-authenticated server endpoint reserved for operator-driven substrate operations — currently `POST /api/v1/admin/planet/create` for share-planet genesis. |
| **ChannelEpoch** | A channel's per-epoch ACC + access grants. |

---

## 14. Deploying amp: topologies, identity & versioning

This section answers the architecture questions that come up when an existing product — with its own auth, its own payments, its own offline story — folds amp in. It describes the **current** model; where something isn't built yet, it says so and gives the bridge.

### 14.1 Deployment topologies — where the vault runs

An `ampd` host is a full peer: it stores, signs, encrypts, and relays. A web / desktop app reaches one in three shapes:

- **Embedded local vault** — bundle `ampd` (or the `libampd` shared library) inside a desktop / Electron app and spawn it as a child process / link it in-process. The user *is* the vault; projects are encrypted on their own machine; reads, writes, and seals work with no network. This is the topology that preserves an **offline-first** product. Cross-compile the Go host per target (darwin-arm64/x64, win-x64/arm64, linux-x64), code-sign each, ship it as a platform resource.
- **Shared cloud vault** — one `ampd` on a server is the rendezvous point for cross-device sync and multi-member collab. Simplest to operate; loses the offline property (the app needs the server reachable to read/write).
- **Hybrid** — an embedded local vault for offline work plus a cloud vault as a sync / relay peer. CRDT writes queue locally and replicate when the cloud peer is reachable. This is the native amp model, not a bolt-on.

A pure-JS / WASM in-process vault is **not** on the near-term path; the embeddable unit today is the native host (`ampd` / `libampd`). For a paid desktop app, **hybrid (embedded + optional cloud sync)** is the recommended shape.

### 14.2 Offline membership & feature gating

Be precise about "offline," because it splits in two:

- **Data is offline-first by construction.** Against a local / embedded vault, reads, writes, and `seal`/`open` all work with no network; writes queue and replicate later.
- **The session token is online-issued.** `login()` returns an opaque random `SessionToken` (a Bearer) minted by the host. It is **not** a self-verifiable signed capability, so a *cloud-only* deployment cannot confirm membership while the network is down.

The way to get the equivalent of a locally-verifiable signed license token is the **embedded local vault** (§14.1): the member's signed membership lives in the local host and verifies locally — no phone-home. The forward path for a *portable* signed capability is the `memberToken` login scheme (`signed(memberID ‖ ts)`, verified against the cached member public key); it is reserved in the wire contract and returns `501 Unsupported` until it ships, so you can lock against it now but not depend on it yet.

For a cloud-only build, **degrade gracefully**: when the host is unreachable and entitlement can't be confirmed, disable the gated action cleanly (grey out "Share") rather than letting it fail mid-flight.

### 14.3 Device-scoped / anonymous identity

Two facts that are easy to conflate:

- **Sealing does not require a human identity.** The device-local `EncryptKey` (§6.2) is generated and persisted independently — `seal`/`open` are a property of *the device*, not of an email / wallet / DID.
- **Persisting to a planet requires a session.** Writing a channel item — even a sealed BYOK blob — is a signed TxMsg against a planet, which needs a logged-in member.

There is **no first-class "issue an anonymous device member" API** today — don't wait on one. To ship BYOK before unifying human identity: keep gating app launch however you do now, log the member into their auto-provisioned home planet with whatever credential you already have, and store sealed secrets under a device-stable item ID. Sealing protects the secret from the host and other members regardless; the login is just what gives you a planet to write to.

### 14.4 Membership tiers, Stripe, and the admin surface

- **Don't put the tier in `member.kind`.** `Kind` is an identity taxonomy (DESIGN-11), not an entitlement, and the protocol never gates on it (§12). Model **payment tier as application data** your app reads (e.g. a `members/billing/{memberID}` item) and enforce capability with **per-channel ACC** — gate `projects.share`, `users.api_keys_overrides`, etc. on the member's access grants.
- **Admin surface that exists today:** `POST /api/v1/admin/credentials/email/issue` (Bearer + the `Admins` allowlist from `app.brand.json`, §10.1) mints an email-scheme member and returns `{ MemberID, Email }`. This is what a Stripe webhook calls server-side to provision a paying customer.
- **Not yet wired:** a tier → ACC *grant / revoke* admin endpoint. When it lands it will extend the `/api/v1/admin/*` surface without a wire-shape break. Until then, write tier as app data on subscription events (create / renew / cancel) and enforce in-app plus via the ACC primitives you have. Don't model entitlement on `Kind` as a stopgap — it's the wrong axis and you'd have to unwind it.

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

---

*This SKILL is maintained by the art.media.platform engineering team. For the protocol primer and primitives, see [`amp.SDK`](https://github.com/art-media-platform/amp.SDK); for the security model, see the companion `SECURITY-amp-web-SDK.md` in this bundle.*
