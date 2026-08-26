# Contributing to amp.SDK

## First build

```bash
go build ./...
```

That is the whole contract. `amp.SDK` is a dependency-light Go library with no
code generation step, no build script, and no `make` target between a fresh
clone and a compile. Requirements: Go (the version in
[`.go-version`](.go-version)) and `git`.

```bash
go test ./...
```

## What lives where

| Path | What it is |
|---|---|
| [`amp/`](amp/) | the wire contract: TxMsg, attrs, planets, epochs, and the `AppModule` interface an app implements |
| [`amp/webapi/`](amp/webapi/) | the canonical HTTP/WebSocket request and response shapes, with golden fixtures |
| [`stdlib/`](stdlib/) | the primitives the contract rests on — `tag` UIDs, `safe` key handling, `task` contexts, `data` publishing |
| [`amp-web/`](amp-web/) | the TypeScript client (`@art-media-platform/web`) and its bundle |
| [`amp-native/`](amp-native/) | the README that ships inside the native kit |

`amp/README.md` is the guide for shipping an AMP channel or channel UI; the
[top-level README](README.md) is the protocol overview.

## Rules that survive review

- **Protobuf for anything on the wire or on disk; Go structs for
  internal-only state.** Field numbers are frozen: add fields, never renumber
  or reuse. Enum fields, never bare bit flags or bools.
- **Regenerate, never hand-edit.** `.pb.go` and `.cs` files come from the
  `.proto` and `.consts.sdl` sources through `make generate` in the amp.planet
  checkout. A hand edit to a generated file is lost on the next regen.
- **Zero-value means unset.** A numeric field's `0` resolves to a named default
  at the point of use, never a baked-in initializer.
- **Names carry their type.** `ID` never `Id`; a UID is a `tag.UID`, not a
  string; an accessor reads `NounByID`.
- **A test must be able to fail.** Assert from the receiving end — that the
  peer observed the effect — not merely that the call returned.

## Changes that reach amp.planet

`amp.planet` depends on a released `amp.SDK` tag, so a change here is not
visible to it until it is tagged, or until that checkout bridges to a local
`amp.SDK` (`make sdk-dev` there). Land the SDK change first, then the
amp.planet side.

## The TypeScript client

`amp-web/` builds and tests on its own:

```bash
cd amp-web && npm ci && npm run build && npm test
```

Its drift guard checks the generated client against the Go wire contract in
`amp/webapi`, so a wire change that skips the client fails there. Bundle
mechanics are in [`amp-web/README.md`](amp-web/README.md).
