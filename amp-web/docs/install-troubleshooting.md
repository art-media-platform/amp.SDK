# Install Troubleshooting

Symptoms seen installing the `amp-web-SDK` bundle, with fixes. Install
steps: README "Install".

## `Cannot find package '@noble/curves'` (or `@noble/hashes`, `@noble/ciphers`)

The bundle directory sits **outside** your project. npm links a `file:`
dependency as a symlink and resolves the package's own deps from its
*real* path — from outside the project that walk-up never reaches your
`node_modules`.

Fix (either):

- Move the bundle inside the project root and reinstall:
  `npm install ./amp-web-SDK`.
- Or give the bundle its own deps once: `cd amp-web-SDK && npm install`.
  A sibling path (`file:../amp-web-SDK`) resolves after that.

## `Cannot find module 'react'` in a Node / Headless Consumer

The package entry imports the React hooks, so `react` must be present
even if you never render — npm >= 7 installs peer dependencies
automatically, but yarn and pnpm do not:

```bash
npm/yarn/pnpm add react   # >= 18
```

## `require() of ES Module … not supported`

The package is ESM-only (`"type": "module"`). Use `import` (or dynamic
`import()` from CommonJS); there is no CJS build.

## Requests Fail Only in the Browser (CORS)

`fetch` to the node works from `curl` but the browser console shows a
CORS error: your web origin isn't on the node's allowlist — an
operator-side setting (SKILL §10). Send your operator the exact origin,
scheme + host + port: `http://localhost:5173` and `http://127.0.0.1:5173`
are **different origins**; dev and prod origins both need listing.

## `refusing insecure WebSocket` Thrown at Login

Your `vaultUrl` is `http://` on a non-loopback host. The session token
rides the WS URL, so the SDK refuses to send it in cleartext. Use
`https://` (the node terminates TLS on 443); plain `http://` is allowed
only to `localhost` / `127.0.0.1` (`SECURITY-amp-web-SDK.md`).

## `401` on Every Call After Login Worked Earlier

The host expired or revoked the session; the SDK drops the persisted
session on any 401 and `useAmpAuth` lands signed out (SKILL §5.1).
Log in again — this is by design, not a bug to work around.

## `403` on Writes, Reads Fine

Your member isn't on the planet the constructor `planetTag` names.
Accept the deploy's invite (SKILL §4.7), or construct with
`planetTag: ''` to use your auto-provisioned home planet.

## Verifying an Install

From inside `amp-web-SDK/` (after `npm install` there):

```bash
npm test    # wire-contract drift guard vs the shipped webapi/testdata fixtures
```

All green means the bundle you hold matches its own wire contract; no
server is contacted.
