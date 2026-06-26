# AMP Forums — example SPA

A functional-complete, responsive forums SPA built on `@art-media-platform/web`.
It re-platforms a phpBB-style board onto an AMP vault: anonymous visitors browse;
logged-in members post, reply, moderate, subscribe, and edit their profile. Every
write is an app-verb invoke (`amp://~/forums/{verb}`) the ampd custodian authors
custodially — the single write path lives in `src/hooks/useForumsApi.ts`.

The design language follows Flarum (re-implemented, not its GPL source).

## Run against a local ampd

1. Start `ampd` as a forums host (founds a public board planet + routes verb
   invokes through its custodian):

   ```
   ampd -forums.host "AMP Forums" -login.unsafe   # -login.unsafe for local dev only
   ```

   At boot it logs `forums custodian ready: board planet <UID> (public)`.

2. Configure the SPA:

   ```
   cp .env.example .env.local
   # set VITE_AMP_PLANET_TAG=<the board planet UID from the log>
   ```

3. Build `@art-media-platform/web` once (this example consumes its `dist/`):

   ```
   cd ../.. && npm install && npm run build && cd examples/forums
   ```

4. Install + run:

   ```
   npm install
   npm run dev      # http://localhost:5174
   npm run build    # production bundle in dist/, later served by ampd's app.www
   ```

## What's here

- `src/forums-attrs.ts` — the single source of the wire contract (channel/attr
  canonics, verb URLs, enum values, item shapes).
- `src/hooks/useForumsApi.ts` — the verb wrappers over `useAmpMutation().invoke`.
- `src/pages/` — board, thread, new-topic, profile, login, pm (placeholder).

Private messages are a later milestone (a public board can't store them
privately); the `/pm` route is a placeholder.
