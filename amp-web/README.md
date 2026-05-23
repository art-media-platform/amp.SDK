# @art-media-platform/web

The **amp-web-SDK** TypeScript client — React hooks + a backend adapter for
building web apps on [art.media.platform](https://github.com/art-media-platform).
Talks the `amp.exe` `app.www` wire contract — defined in
[`amp.SDK/amp/webapi`](https://github.com/art-media-platform/amp.SDK/tree/main/amp/webapi),
and copied into this SDK bundle at [`webapi/webapi.types.go`](webapi/webapi.types.go).

> **Beta (pre-v400).** Distributed in the `amp-web-SDK` bundle — not yet on npm.
> Get the latest by building it (`make amp-web-SDK`) or from your amp contact. At
> v400 (Production Ready) it flips to public GitHub Releases + an open license.

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

// 1. Point the client at your amp node + planet.
const client = new AmpWebClient({
  vaultUrl:  import.meta.env.VITE_AMP_VAULT_URL,   // e.g. https://my-amp-node:5193
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

  if (loading) return <p>Loading…</p>;
  return (
    <ul>
      {data.map(label => <li key={label._itemID}>{label.title}</li>)}
      <button onClick={() => create('projects', 'labels', { title: 'New' })}>Add</button>
    </ul>
  );
}
```

### Login

```tsx
const { login } = useAmpAuth();

// Wallet sign-in (EIP-4361 / SIWE — any EVM wallet): connect, fetch challenge, sign, submit.
const address   = await connectWallet();                       // your wallet picker (e.g. EIP-6963)
const challenge = await client.getWalletChallenge(address);    // { nonce, message }
const signature = await signWithWallet(challenge.message);     // wallet personal_sign
await login({ scheme: 'wallet', address, signature, nonce: challenge.nonce });

// W3C DID sign-in (login-only): did:key (Ed25519) or did:pkh:eip155 (EVM wallet).
const ch  = await client.getDIDChallenge(did);                 // { nonce, message }
const sig = await signChallenge(did, ch.message);              // ed25519, or wallet personal_sign for did:pkh:eip155
await login({ scheme: 'did', did, signature: sig, nonce: ch.nonce });

// Email / password:
await login({ scheme: 'email', email, password });
```

> A `did:pkh:eip155` login resolves to the **same member** as a `wallet` login over that address. DID here is authentication only — not Verifiable Credentials.

## Hooks

| Hook | Purpose |
|------|---------|
| `useAmpAuth()` | Login, logout, auth state |
| `useAmpQuery<T>(channel, attr, opts?)` | Read items + live WebSocket subscription |
| `useAmpMutation()` | `tx` (batched), `create`, `upsert`, `remove`, `withdraw` |
| `useAmpUpload()` | Blob upload with progress |
| `useAmpMedia(blobRefID)` | Resolve a blob to a streamable URL |
| `useAmpCrypto()` | Sealed-box BYOK — `seal` / `open` against the session EncryptKey |

The canonical write is `tx(ops)` — one TxMsg, N atomic ops, one signature.
`create` / `upsert` / `remove` / `withdraw` are single-op convenience wrappers.

## Canonic names → tag.UIDs

`(channel, attr, itemID)` are `tag.UID`s derived from string names by a
non-trivial canonicalization. Don't reimplement it — two supported paths:

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
