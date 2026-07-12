# AMP Forums — example SPA

A functional-complete, responsive forums SPA built on `@art-media-platform/web`.
It re-platforms a phpBB-style board onto an AMP vault: anonymous visitors browse;
logged-in members post, reply, moderate, subscribe, and edit their profile. Every
write is an app-verb invoke (`amp://~/forums/{verb}`) the ampd custodian authors
custodially — the single write path lives in `src/hooks/useForumsApi.ts`.

The design language follows Flarum (re-implemented, not its GPL source).

## Run Against an Operated Node

You connect to an `ampd` node an operator runs — you do not run one
(SKILL §0). Ask your amp contact for the node URL and the board planet tag.

1. Make the SDK package resolvable. The example depends on `file:../..` —
   the package root two levels up:

   - **Bundle view** (`amp-web-SDK/examples/forums`): `dist/` ships prebuilt;
     run `npm install` once at the bundle root so its `@noble/*` deps resolve.
   - **Repo view** (`amp.SDK/amp-web/examples/forums`): build the client
     once — `cd ../.. && npm install && npm run build`.

2. Configure the SPA:

   ```
   cp .env.example .env.local
   # VITE_AMP_VAULT_URL=<the operated node, e.g. https://prod.plan.tools>
   # VITE_AMP_PLANET_TAG=<the board planet tag or UID, handed to you>
   ```

3. Install + run:

   ```
   npm install
   npm run dev      # http://localhost:5174
   npm run build    # production bundle in dist/, later served by ampd's app.www
   ```

Your dev origin (`http://localhost:5174`) must be on the node's CORS
allowlist — an operator-side setting (SKILL §10). Ask for it when you get
the planet tag.

## Run Against a Local ampd (AMP-Internal / Self-Hosted Dev)

Only if you build `ampd` yourself — not part of a web-SDK integration
(SKILL §14.8):

1. Start `ampd` as a forums host (founds a public board planet + routes verb
   invokes through its custodian):

   ```
   ampd -forums.host "AMP Forums" -login.unsafe   # -login.unsafe for local dev only
   ```

   At boot it logs `forums custodian ready: board planet <UID> (public)`.

2. Point `.env.local` at it: `VITE_AMP_VAULT_URL=http://localhost:5193`,
   `VITE_AMP_PLANET_TAG=<the board planet UID from the log>`.

## What's Here

- `src/forums-attrs.ts` — the single source of the wire contract (channel/attr
  canonics, verb URLs, enum values, item shapes).
- `src/hooks/useForumsApi.ts` — the verb wrappers over `useAmpMutation().invoke`.
- `src/pages/` — board, thread, new-topic, profile, login, pm (placeholder).

Private messages are a later milestone (a public board can't store them
privately); the `/pm` route is a placeholder.
