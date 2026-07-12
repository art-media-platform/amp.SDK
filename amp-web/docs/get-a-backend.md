# Get a Backend

Every call this SDK makes goes to an `ampd` node over HTTPS. You never
build, run, or host that node for a web integration (SKILL §0) — so the
first question is where yours comes from. Four answers, in the order you
should try them.

## 1. You Were Handed One (the Partner Default)

Your amp contact hands you three things:

| Item | Example | Where it goes |
|---|---|---|
| Node URL | `https://prod.plan.tools` | `vaultUrl` / `VITE_AMP_VAULT_URL` |
| Planet tag | `my-app-planet` | `planetTag` / `VITE_AMP_PLANET_TAG` |
| Invite URL + passphrase | `https://{fqdn}/invite#…` | `client.acceptInvite(…)` after login (SKILL §4.7) |

Also tell your contact the web origins your app runs on (e.g.
`http://localhost:5173`, `https://myapp.example.com`) — the node's CORS
allowlist is operator-side (SKILL §10), and a missing origin looks like a
network failure in the browser console.

With the node URL alone you can already log in and round-trip data
(next section); the planet tag and invite scope you to the deploy's
planets and federation.

## 2. Self-Provision: Wallet Login Mints Your Planet

No planet tag yet? Wallet (SIWE) login **auto-provisions a private home
planet** on the node — zero operator setup:

```ts
const client = new AmpWebClient({ vaultUrl, planetTag: '' });
// wallet login (docs/siwe-primer.md) …
await client.create('hello', 'world', { msg: 'hi' });   // lands on YOUR planet
```

An empty `planetTag` targets the session's own default planet. This is
the fastest path to a working round-trip — prove the pipe first, join
the deploy's federation second (`acceptInvite`, SKILL §4.7).

Email-scheme members, by contrast, are **operator-issued** (an admin
verb mints them — SKILL §14.4); you can't self-provision one.

## 3. A Development Node

For local development against a server you control, ask your amp contact
for a dev `ampd` build (a single native binary; you run it, you don't
build it). Typical local run for the forums example:
`examples/forums/README.md`. Local dev is plain HTTP on
`http://localhost:5193`; the SDK allows cleartext only to loopback.

There is no hosted public sandbox today — the dev binary or a handed
node are the two development paths.

## 4. Run Your Own Node (the Licensee Path)

Operating your own `ampd` — your own planets, CORS, admin — and
federating it with a parent deploy is the heavier, self-hosting path:
SKILL §14.8. It is never required for a web-SDK integration.

## Time to First Success

With a handed node URL: minutes — install the bundle (README), wallet
login, one `create`, one `query`. Everything after that (invites,
share planets, cards) is incremental.
