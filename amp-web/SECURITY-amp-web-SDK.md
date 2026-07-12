# Security model — amp-web-SDK

What a web app built on amp can rely on, and the handful of things the SDK
**cannot** do for you. This is the consumer-facing summary; the protocol
primer lives in the [amp.SDK README](https://github.com/art-media-platform/amp.SDK).

---

## What a vault can and cannot see

A vault (an `ampd` relay peer) stores and forwards encrypted transactions.
On a **private** planet it can verify that a transaction came from a legitimate
member — via an HMAC membership proof derived from the epoch key — **without
decrypting the content or learning which member authored it**.

| A vault sees | A vault does NOT see (private planet) |
|---|---|
| PlanetID, EpochID | message payloads (encrypted) |
| activity timeline (when, how much) | who authored an encrypted tx it relays (only an HMAC proof) |
| membership-proof validity | private-channel keys it isn't a member of |

A seized vault yields opaque blobs and proofs — no plaintext. Routing metadata
(PlanetID, EpochID, timing) is observable, and a sealed value still leaks its
**size** (sealed-box adds a fixed overhead, so ciphertext length ≈ plaintext length)
and its channel/attr/item address; treat that as your metadata surface. Sealing
hides content, not that a secret of ~N bytes exists in slot X and changed at time T.
(Note this is the *relay* layer — when the host serves you decrypted items/frames,
each carries `_FromID`, the author; see Attribution below.)

---

## Encryption

- **Private planet** — every transaction is encrypted under the planet's current
  **epoch key**. Only members holding that key decrypt. Removing a member rotates
  the epoch; the revoked member never receives the new key.
- **Public / share planet** — `PlanetEpoch.IsPublic = true` at genesis: signed but
  not encrypted, anonymous-readable, member-writable. This is the *only* mode that
  allows anonymous reads. amp has **no per-item visibility flag** — public content
  lives on a separate public planet (SKILL §6.4).
- Encrypted blobs are decrypted on demand by the host and served as plaintext over
  the (TLS-terminated) wire. **Plaintext is never persisted to vault disk.**

---

## Identity & login

- MemberID is derived from a canonic identity URI: `eth:<lowercase-addr>` for the
  wallet scheme, `email:<lowercase-addr>` for email — deterministic and stable
  across sessions, so the same identity always resolves to the same MemberID.
- **Email scheme is timing-uniform.** Wrong-password and unknown-email both return
  one `401 / AuthFailed` envelope, and the password KDF runs on every attempt — the
  response carries **no existence oracle**.
- **Recovery is 202-uniform.** `POST /api/v1/login/email/recover` returns `202`
  whether or not the email is bound to a member; the magic-link email is the only
  channel that signals existence (to the real inbox owner).

---

## Session tokens in URLs

Two surfaces carry the session Bearer in a URL query string rather than an
`Authorization` header:

- **`wss://…/ws?token=<sessionToken>`** — browsers can't set headers on a WebSocket
  upgrade, so the token rides the query. `wss://` hides it from network observers, but
  it still lands in **server access logs**. Don't log WS URLs.
- **`{vaultUrl}/cards/{cell}?token=<sessionToken>&planet=…`** — a card URL is a
  *navigable* WebView / iframe `src`, so the Bearer there can leak via the `Referer`
  header (to any third-party resource the card loads), browser history, and access logs.

The Bearer authorizes the member's full read / write / invite surface, so treat these
URLs as secrets: require `https://` / `wss://` (never plaintext), never log them, and
don't load third-party resources in a card you wouldn't hand the token to. A
shorter-lived, capability-scoped card token is the intended direction.

---

## Session persistence — the Bearer at rest

The SDK persists the session (Bearer token + member record) in IndexedDB, keyed
by vault URL, so a page reload rehydrates instead of logging the member out.
The stored token is re-validated against `GET /api/v1/session` before it is
trusted; a host-rejected token is cleared on the spot.

What that storage means:

- **Same trust boundary as the device EncryptKey** (below): the token is
  readable by any same-origin JavaScript and by a local-device compromise.
  An XSS payload that can read it holds the member's full session until it
  expires or is revoked — your app's CSP / XSS posture is part of the session
  trust boundary. Ship a strict CSP.
- **The server bounds the blast radius, not the client.** The token is an
  opaque 128-bit random Bearer with a host-side expiry (`ExpiresAt`); logout
  drops it server-side, and the SDK clears the persisted copy on `logout()`
  and on any 401. A JS string cannot be zeroized in place — expiry and
  revocation are the real backstops.
- **No key material rides with it.** The session record is the Bearer + public
  member facts; the EncryptKey lives in its own store and never enters the
  session record.
- **Opting out**: inject `sessionStore: new MemorySessionStore()` — sessions
  then live for the tab only, and a reload requires a fresh login. Consider it
  for shared-terminal deployments.

---

## Sealed-box BYOK — the #1 rule

User-supplied secrets (API keys for Cesium Ion, OpenRouter, Mapbox, …) **must be
sealed client-side before any `upsert`**. A plaintext secret in a channel item is
readable by *every member* of the planet who holds the epoch key, and a memory
scrape of the local key cache exposes it after the fact.

```ts
// Channel items are JSON — base64-encode the sealed bytes (a raw Uint8Array
// does not survive JSON.stringify).  Decode with base64ToBytes before open().
const sealed = bytesToBase64(await client.seal(new TextEncoder().encode(plaintext)));
await client.upsert('users', 'api_keys_overrides', member.ID, { cesium: sealed });
```

`seal`/`open` wrap anonymous-sender HPKE (default kit Poly25519, pure JS) and are
byte-compatible with the Go side. **Plaintext API keys in a channel item are the
single most common security mistake — never do it.**

**A failed `seal()` must abort the write — never fall back to plaintext.** `login`
installs the EncryptKey best-effort; if device storage is unavailable (private-mode
quota, a blocked IndexedDB upgrade), BYOK is left uninstalled and `seal()` throws.
That is fail-closed *only if you let it fail*: a `catch` that stores the secret
unsealed turns a transient storage error into a silent plaintext leak. Gate the write
on `client.getEncryptPub() !== null`, or let `seal()`'s throw propagate and abort.

**The EncryptKey is device-local.** The client generates a per-member EncryptKey
on first login and persists it in the browser (IndexedDB), reinstalling it on every
later login — so `seal`/`open` work for any logged-in member with no out-of-band
setup. The private key never reaches the host or any other member, so a sealed
secret stays opaque to admins, vault operators, other planet members, and a
cold-store / forensic adversary holding the planet epoch key. That is the
property the planet epoch key alone cannot give you.

Two consequences to design around:

- **At rest & same-origin.** The private key sits in IndexedDB as raw bytes — no
  WebCrypto non-extractable handle, no passphrase wrap. So it is readable by (a) a
  local-device compromise (disk forensics, a hostile extension) **and (b) any
  same-origin JavaScript** — an XSS payload in *your own app* exfiltrates it with no
  device compromise at all. Your app's CSP / XSS posture is therefore part of the BYOK
  trust boundary: BYOK protects against the *host* and *other members*, not against
  script you let run on your origin, nor a compromised device. Ship a strict CSP.
- **Scope is same-device.** Because the key is device-local, "seal on phone, open on
  laptop" does **not** work: a member who clears storage or signs in on a second
  device gets a fresh key there and re-enters their (re-enterable) BYOK secrets.
  Cross-device recovery would require the host to carry a key wrapped under a secret
  it never sees (a client-derived KDF key) — a separate mechanism, deliberately not
  built, until a use case needs it.

To override the auto-installed key (e.g. derive it from a wallet), call
`client.setEncryptKey(...)` after login.

---

## Attribution & anonymity (your responsibility, not the wire's)

- **`fromID` rides every read, subscribe, and withdraw frame** — it names the member
  who authored the op. On a share planet that is the author's identity. **Anonymity
  is an app-layer policy**, not a wire guarantee: if your UI must not reveal authors,
  don't render `fromID`.
- **Share-planet attribution stripping is app-layer.** When you copy content to a
  public share planet, strip owner attribution by default unless the owner opts in.
- **Never persist ephemeral credentials in channels** (collab room tokens, short-lived
  service tokens). They're session-scoped; keep them out of the CRDT.

---

## Telemetry stays local

The server **never** pushes security telemetry (failed-login, rate-limit, audit)
over the consumer WebSocket, and your app must not replicate SecurityEvent-shaped
data into a channel. Sharing audit telemetry creates a metadata oracle. Keep it
device-local.

---

## Operator notes (deploy-side)

- **CORS:** `AllowedOrigins: ["*"]` together with credentials echoes any origin back —
  a footgun. List specific origins in `app.brand.json`.
- **Protect `app.brand.json`** — it carries the admin UID allowlist; keep it
  file-ACL-restricted.
- Run behind a TLS terminator in production. The plaintext-over-wire guarantee
  assumes the transport is TLS.

**Client-side:** `vaultUrl` MUST be `https://` in any non-localhost deployment. The
SDK derives `wss://` from it and **refuses** a cleartext `ws://` upgrade to any
non-loopback host (the Bearer rides the WS URL). REST traffic has no such guard —
an `http://` vaultUrl sends the Bearer header in the clear — so your `vaultUrl`
config remains the control point.
