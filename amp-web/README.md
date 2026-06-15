# @art-media-platform/web

The **amp-web-SDK** TypeScript client — React hooks + a backend adapter for
building web apps on [art.media.platform](https://github.com/art-media-platform).
Talks the `ampd` `app.www` wire contract — defined in
[`amp.SDK/amp/webapi`](https://github.com/art-media-platform/amp.SDK/tree/main/amp/webapi),
and copied into this SDK bundle at [`webapi/webapi.types.go`](webapi/webapi.types.go).

> **Beta (pre-v400).** Distributed in the `amp-web-SDK` bundle — not yet on npm.
> Get the latest by building it (`make amp-web-SDK`) or from your amp contact. At
> v400 (Production Ready) it flips to public GitHub Releases + an open license.

> **You connect to an operated node — you do not run one.** The vault (`ampd`) is
> server software an operator runs; this package is a browser / Node **client**
> that talks to it over HTTPS. For Maplable that node is `https://prod.plan.tools`.
> Building a Go server means you're off the path — see `SKILL-amp-web-SDK.md` §0.

## Install

Install from the client directory shipped in the bundle — it resolves under its
package name, `@art-media-platform/web`:

```bash
npm install ./amp-web-SDK
```

Or pin it as a path dependency in your `package.json`:

```json
{ "dependencies": { "@art-media-platform/web": "file:./amp-web-SDK" } }
```

Peer dependency: `react` (>= 18). No other runtime deps — the sealed-box crypto
is pure JS (`@noble/*`); networking is native `fetch` + `WebSocket`.

## Quick Start

```tsx
import { AmpProvider, AmpWebClient, useAmpQuery, useAmpMutation } from '@art-media-platform/web';

// 1. Point the client at the operated amp node + your planet (you don't run the node).
const client = new AmpWebClient({
  vaultUrl:  import.meta.env.VITE_AMP_VAULT_URL,   // operated node — e.g. https://prod.plan.tools
  planetTag: import.meta.env.VITE_AMP_PLANET_TAG,  // the planet your app reads/writes
});

// 2. Wrap your app.
function App() {
  return (
    <AmpProvider client={client}>
      <Labels />
    </AmpProvider>
  );
}

// 3. Use hooks (reads auto-subscribe over WebSocket).
function Labels() {
  const { data, loading } = useAmpQuery<{ title: string }>('projects', 'labels');
  const { create } = useAmpMutation();
  const { isAuthenticated } = useAmpAuth();   // reads can be anonymous; writes require login (see § Login)

  if (loading) return <p>Loading…</p>;
  return (
    <ul>
      {data.map(label => <li key={label._ItemID}>{label.title}</li>)}
      {/* writes 401 without a session — gate the control on auth */}
      <button disabled={!isAuthenticated} onClick={() => create('projects', 'labels', { title: 'New' })}>Add</button>
    </ul>
  );
}
```

### Login

```tsx
const { login } = useAmpAuth();
const client    = useAmpClient(); // the adapter, for challenge fetches below

// Wallet sign-in (EIP-4361 / SIWE — any EVM wallet): connect, fetch challenge, sign, submit.
const address   = await connectWallet();                       // your wallet picker (e.g. EIP-6963)
const challenge = await client.getWalletChallenge(address);    // { Nonce, Message }
const signature = await signWithWallet(challenge.Message);     // wallet personal_sign
await login({ Scheme: 'wallet', Address: address, Signature: signature, Nonce: challenge.Nonce });

// W3C DID sign-in (login-only): did:key (Ed25519) or did:pkh:eip155 (EVM wallet).
const ch  = await client.getDIDChallenge(did);                 // { Nonce, Message }
const sig = await signChallenge(did, ch.Message);              // ed25519, or wallet personal_sign for did:pkh:eip155
await login({ Scheme: 'did', DID: did, Signature: sig, Nonce: ch.Nonce });

// Email / password:
await login({ Scheme: 'email', Email: email, Password: password });
```

> A `did:pkh:eip155` login resolves to the **same member** as a `wallet` login over that address. DID here is authentication only — not Verifiable Credentials.

### Joining a federation

Handed an `amp-invite-v1:…` token? After login, `await client.acceptInvite({ inviteText, passphrase })` joins you to that federation (passphrase arrives out-of-band). You don't need it to read/write your *own* planet, but **registering planets / names into a federation requires membership** — which the invite grants. See `SKILL-amp-web-SDK.md` §4.7.

## Hooks

| Hook | Purpose |
|------|---------|
| `useAmpAuth()` | Login, logout, auth state |
| `useAmpClient()` | The adapter for imperative calls — login challenges, `resolveTag`, ad-hoc `query` |
| `useAmpQuery<T>(channel, attr, opts?)` | Read items + live WebSocket subscription |
| `useAmpMutation()` | `tx` (batched), `create`, `upsert`, `remove`, `withdraw` |
| `useAmpUpload()` | Blob upload with progress |
| `useAmpMedia(blobUID)` | Resolve a blob to a streamable URL |
| `useAmpCrypto()` | Sealed-box BYOK — `seal` / `open` against the session EncryptKey |

The canonical write is `tx(ops)` — one TxMsg, N atomic ops, one signature.
`create` / `upsert` / `remove` / `withdraw` are single-op convenience wrappers.

## Canonic names → tag.UIDs

`(channel, attr, itemID)` are `tag.UID`s derived from string names by a
non-trivial canonization. Don't reimplement it — two supported paths:

- **Build-time:** generate canonic `TagName`/`TagUID` constants with
  [`forge`](https://github.com/art-media-platform/forge) from your `.sdl` keys, so
  your app never registers known keys at runtime.
- **Runtime:** `client.resolveTag(expr)` / `client.resolveTags(exprs)` resolve
  names server-side (anonymous, long-cacheable) — cache the results.

## Full API reference

- **[`SKILL-amp-web-SDK.md`](SKILL-amp-web-SDK.md)** — the complete SDK contract:
  wire shape, data-model conventions, sealed-box BYOK, card / WebRect bridge.
- **[`SECURITY-amp-web-SDK.md`](SECURITY-amp-web-SDK.md)** — the security model a
  consumer relies on (encryption, identity, what a vault can and cannot see).

## Versioning & stability

**Beta, pre-v400.** Breaking changes are possible between minor revisions, and
there is no public npm channel yet — you integrate a versioned bundle
(`amp-web-SDK-vNNN[.P].zip`). The bundle label, this package's `version`, and the
`amp.SDK` release tag are the **same revision** — one source of truth. **Pin the
bundle you integrated** (vendor it, or commit the version) rather than floating;
upgrade deliberately and re-run your contract tests. At v400 ("Production Ready")
the SDK moves to public GitHub Releases under an open license with a semver
compatibility promise.

Deployment topologies, offline-auth, device identity, and membership-tier
guidance live in [`SKILL-amp-web-SDK.md`](SKILL-amp-web-SDK.md) §14.
