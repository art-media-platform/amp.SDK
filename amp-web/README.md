# @art-media-platform/web

The **amp-web-SDK** TypeScript client — React hooks + a backend adapter for
building web apps on [art.media.platform](https://github.com/art-media-platform).
Talks the `ampd` `app.www` wire contract — defined in
[`amp.SDK/amp/webapi`](https://github.com/art-media-platform/amp.SDK/tree/main/amp/webapi),
and copied into this SDK bundle at `webapi/webapi.types.go`.

> **Beta.** Distributed in the `amp-web-SDK` bundle — not yet on npm.
> Get the latest from your amp contact, or build it from an `amp.SDK`
> checkout: `amp-web/pack.sh` → `dist/amp-web-SDK-vNNN[.P].zip`.
> AMP intends to move to public releases under an open license when resources
> allow and the business is more stable; no timeline is committed.

> **You connect to an operated node — you do not run one.** The vault (`ampd`) is
> server software an operator runs; this package is a browser / Node **client**
> that talks to it over HTTPS. For Maplable that node is `https://prod.plan.tools`.
> Building a Go server means you're off the path — see `SKILL-amp-web-SDK.md` §0.

## Install

Unzip the bundle **inside your project directory** and install it by path — it
resolves under its package name, `@art-media-platform/web`:

```bash
unzip amp-web-SDK-vNNN.zip   # → ./amp-web-SDK
npm install ./amp-web-SDK
```

Or pin it as a path dependency in your `package.json`:

```json
{ "dependencies": { "@art-media-platform/web": "file:./amp-web-SDK" } }
```

Two install traps to know up front (fixes and symptoms in
[`docs/install-troubleshooting.md`](docs/install-troubleshooting.md)):

- **Keep the bundle inside your project.** npm links a path dependency as a
  symlink; from a directory outside the project the bundle's `@noble/*`
  runtime deps do not resolve through the link (`Cannot find package
  '@noble/curves'`). If the bundle must live elsewhere, run `npm install`
  once inside `amp-web-SDK/` so it carries its own `node_modules`.
- **`react` (>= 18) is required even headless.** The package entry imports
  the React hooks, so Node / SSR consumers need `react` installed too.
  npm >= 7 installs peer dependencies automatically; with yarn or pnpm add
  `react` explicitly.

Runtime deps are three pure-JS `@noble/*` packages (the sealed-box crypto);
networking is native `fetch` + `WebSocket`.

## Quick Start

```tsx
import { AmpProvider, AmpWebClient, useAmpAuth, useAmpClient, useAmpQuery, useAmpMutation } from '@art-media-platform/web';

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

// 3. Use hooks (logged-in reads auto-subscribe over WebSocket;
//    anonymous reads are one-shot fetches — no live updates without a session).
function Labels() {
  const { data, loading } = useAmpQuery<{ title: string }>('projects', 'labels');
  const { create } = useAmpMutation();
  const { isAuthenticated } = useAmpAuth();   // private-planet reads & writes both require login; anonymous reads need a public share planet (SKILL §6.4)

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

`connectWallet` / `signWithWallet` / `signChallenge` below are **your** wallet glue (EIP-1193 / Ed25519), not SDK exports — see `SKILL-amp-web-SDK.md` §5.1 for the full implementation.

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

### Joining a Federation

Handed an invite URL (`https://{fqdn}/invite#…`)? After login, `await client.acceptInvite({ inviteText, passphrase })` joins you to that federation (passphrase arrives out-of-band). You don't need it to read/write your *own* planet, but **registering planets / names into a federation requires membership** — which the invite grants. Operators mint them with `client.issueInvite({ planet, passphrase, maxRedemptions })` (one URL, many self-minting members), revoke with `client.revokeInvite`, and audit with `client.listInvites`. See `SKILL-amp-web-SDK.md` §4.7.

## Hooks

| Hook | Purpose |
|------|---------|
| `useAmpAuth()` | Login, logout, auth state |
| `useAmpClient()` | The adapter for imperative calls — login challenges, `resolveTag`, ad-hoc `query` |
| `useAmpQuery<T>(channel, attr, opts?)` | Read items + live WebSocket subscription (live once logged in) |
| `useAmpMutation()` | `tx` (batched), `invoke` (verb-RPC), `create`, `upsert`, `remove`, `withdraw` |
| `useAmpUpload()` | Blob upload; `progress` signals completion (0 → 100), not streamed percent |
| `useAmpMedia(blobUID, planetTag?)` | Resolve a blob to a streamable URL |
| `useAmpCrypto()` | Sealed-box BYOK — `seal` / `open` against the session EncryptKey |

The canonical write is `tx(ops)` — one TxMsg, N atomic ops, one signature.
`create` / `upsert` / `remove` / `withdraw` are single-op convenience wrappers.

**Error convention:** `useAmpQuery` surfaces failures via `error` state; action
hooks (`useAmpMutation` / `useAmpUpload`) set `error` **and** throw, so you can
`try/catch` an awaited call. `useAmpMedia` is the exception: a failed resolve
falls back to the direct `/www/{UID}` stream URL and its `error` stays null —
a truly missing blob surfaces on the media element, not the hook.

## Canonic Names → tag.UIDs

`(channel, attr, itemID)` are `tag.UID`s derived from string names by a
non-trivial canonization. Don't reimplement it — two supported paths:

- **Build-time:** generate canonic `TagName`/`TagUID` constants with
  [`forge`](https://github.com/art-media-platform/forge) from your `.sdl` keys, so
  your app never registers known keys at runtime. The platform's own
  vocabulary ships pre-generated: `import { std, safe } from
  '@art-media-platform/web'` (UIDs byte-matched to the Go/C# consts).
- **Runtime:** `client.resolveTag(expr)` / `client.resolveTags(exprs)` resolve
  names server-side (anonymous, long-cacheable) — cache the results.

## Example App

[`examples/forums/`](examples/forums/) — a functional-complete forums SPA on
this SDK: anonymous browse, invoke-based writes, login, live subscriptions.
The best walk-through of the SDK in real use; its README covers running it
against an operated node.

## Verifying the Bundle

`npm test` (inside `amp-web-SDK/`, after `npm install`) runs the SDK's own
suite, including the wire-contract drift guard against the golden fixtures
shipped in `webapi/testdata/` (repo: `amp/webapi/testdata/`) — no server
needed. `webapi/webapi.types.go` is the wire contract those fixtures pin.

> **Bundle view vs repo view.** `webapi/` and `AOM/` exist only in the
> distributed bundle — `pack.sh` stages them from `amp.SDK/amp/webapi` and the
> AOM allowlist. In an `amp.SDK` checkout, `amp-web/` has neither; the tests
> probe both locations, so `npm test` passes from either view.

## Full API reference

- **[`SKILL-amp-web-SDK.md`](SKILL-amp-web-SDK.md)** — the complete SDK contract:
  wire shape, data-model conventions, sealed-box BYOK, card / WebRect bridge.
- **[`SECURITY-amp-web-SDK.md`](SECURITY-amp-web-SDK.md)** — the security model a
  consumer relies on (encryption, identity, what a vault can and cannot see).

## Onboarding Docs

Shipped in [`docs/`](docs/) — read in this order when starting cold:

- **[`docs/get-a-backend.md`](docs/get-a-backend.md)** — where your `ampd` node and planet come from (the question everything else assumes answered).
- **[`docs/concepts-primer.md`](docs/concepts-primer.md)** — planet → node → attr → item → edit, and what "channel" precisely means.
- **[`docs/first-channel.md`](docs/first-channel.md)** — from-zero tutorial: login, write, watch it update live.
- **[`docs/siwe-primer.md`](docs/siwe-primer.md)** — wallet (SIWE) login without the web3 homework, plus a drop-in `useWalletLogin()`.
- **[`docs/install-troubleshooting.md`](docs/install-troubleshooting.md)** — symptoms + fixes for the known install traps.

## Design References (AOM)

The distributed bundle ships a curated set of design docs from AMP's
**Operations Manual (AOM)** under `AOM/` — background reading for deeper
architectural context (bundle view only; in the `amp.SDK` repo these live in
the private `amp.planet`). They are **not the contract**: this README,
[`SKILL-amp-web-SDK.md`](SKILL-amp-web-SDK.md), and `webapi/webapi.types.go` are.
**[`docs/aom-index.md`](docs/aom-index.md)** is the reading guide — start at
`AOM/DD-architecture-overview.md`, then dip in as questions arise.

> References marked (internal) name AMP-internal design docs not shipped in this bundle — background provenance, not required reading.

### Authoring Notes

`pack.sh` runs a de-link pass over every staged `.md` (`pack-delink.mjs`): a
relative link whose target ships in the bundle is kept; a link to an AOM doc
outside the bundle allowlist is rewritten to the greppable token
`Text (internal)`; any other dangling relative link fails the pack (file:line).
The legend line above must stay in this section — the pack verifies it.

Rules the pack enforces on partner-visible text:

- "Prereading:" / "must read" lists name shipped docs only. A line that
  instructs the reader to read a doc (`preread`, `must read`,
  `required reading`, `read … first`) while citing one that does not ship —
  by link or by code span — fails the pack; the `(internal)` token cannot
  rescue an instruction; reword at source.
- Cite an internal doc as background, never instruction:
  `` Background (internal): `XYZ.md`. ``

## Versioning & Stability

**Beta.** Breaking changes are possible between minor revisions, and
there is no public npm channel yet — you integrate a versioned bundle
(`amp-web-SDK-vNNN[.P].zip`). The `amp.SDK` git tag is the single source of truth:
`pack.sh` stamps the bundle label and the packaged `package.json` version from it
at build time (the committed `package.json` carries only the `0.0.0-dev`
placeholder). **Pin the bundle you integrated** (vendor it, or commit the version)
rather than floating;
upgrade deliberately and re-run your contract tests. AMP intends to move the SDK
to public releases under an open license, with a semver compatibility promise,
when resources allow and the business is more stable.

Deployment topologies, offline-auth, device identity, and membership-tier
guidance live in [`SKILL-amp-web-SDK.md`](SKILL-amp-web-SDK.md) §14.
