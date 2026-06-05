# `amp/` — Build a Channel

This package is the **channel runtime** of [art.media.platform](../README.md).  If the [root README](../README.md) is the *why* (federated, end-to-end-encrypted, no central server), this is the *how*: the handful of interfaces a third-party software company implements to ship its own channel on top of AMP — and inherit encryption, sync, signing, CRDT merge, offline operation, asset streaming, and federation for free.

You write one Go type.  You do **not** write a server, a database, a sync protocol, a key-exchange, a websocket layer, or an auth system.  Those tools are already shipping as of v300.

---

## What is a Channel?

A **channel** is a `(NodeID, AttrID)` address space with a behavior contract.

- **`NodeID`** — a node within a planet (the "where").
- **`AttrID`** — a typed attribute on that node (the "what"): a proto.Message schema you register.
- **Behavior** — what your code does when a client *pins* (subscribes to) that address: what state it serves, and how it handles writes.

Every piece of state in AMP has an [`amp.Address`](../stdlib/tag/api.tag.go) — `planet → node → attr → item → edit`.  A channel is the slice of that address space your app owns.  Writes are [`TxMsg`](api.app.go) transactions of CRDT ops; two members editing the same item offline converge automatically on sync.

> **Addressing:.**  A `tag.UID` is `[2]uint64` — **16 bytes**.  An [`Address`](../stdlib/tag/api.tag.go) is built from them: `NodeID · AttrID · ItemID · EditID`.  So targeting one independently-addressed, access-controlled, individually-encryptable cell costs **80 bytes** on the wire, plus a compact AEAD envelope.  You spend UIDs like they're free — because they very nearly are.  A planet carries *billions* of channels and items before the address space is ever what runs out.

> **Vocabulary:** "Channel" is everyday shorthand.  The addressing primitive is always `(NodeID, AttrID)` with a behavior — keep that exact when it matters; stay casual when it doesn't.

---

## One Interface

A channel is an [`AppModule`](api.app.go) registered against an invocation tag.  When a client pins a URL that resolves to your tag, the runtime spins up an [`AppInstance`](api.app.go) and asks it to serve [`Pin`](api.app.go)s:

```
client pins  amp://{planet}/notes/...
        │
        ▼
amp.Host ──► FindModule(tag) ──► NewAppInstance(ctx) ──► AppInstance
                                                              │
                       MakeReady(req)  ◄── validate / auth ───┤
                       StartPin(req)   ◄── one per request ───┘
                              │
                              ▼
                         Pin  ── serves (NodeID, AttrID) state ──► TxMsg ──► client
                              ◄── client writes (TxMsg) ──────────────────────┘
```

| Step | You Implement | Runtime Gives You |
|------|---------------|-------------------|
| **Register** | `RegisterWith(reg)` → `RegisterModule(&AppModule{…})` | Tag resolution, invocation routing |
| **Instantiate** | `NewAppInstance(ctx)` — return your struct | `AppContext`: session, env, `NewTx()`, asset publishing |
| **Validate** | `MakeReady(req)` — refresh tokens, check access | Verified signer identity, planet membership |
| **Serve** | `StartPin(req)` — return a `Pin` | Encryption, sync, fanout, CRDT merge, offline queue |
| **Tear down** | `OnClosing()` | Whole-subtree close via `task.Context` |

Every long-lived object is a node in a [`task.Context`](../stdlib/task/api.task.go) tree.  Close a parent, every child Pin closes with it — no goroutine leaks, no manual bookkeeping.

---

## Channel Conventions

Most channels are one of two shapes.  The [`amp/std`](std/) package gives you a skeleton for both via `std.AppModule[*yourApp]`.

### 1. Send-Only — Serve State to Clients

For listings, catalogs, exports, read models.  Implement [`std.Item`](std/api.std.go) and hand it to `PinAndServe`; the runtime walks your item tree, serializes attrs, and pushes a synced `TxMsg`.

```go
func (app *appInst) StartPin(req *amp.Request) (amp.Pin, error) {
    return app.PinAndServe(&notesRoot{}, req)
}

type notesRoot struct {
    std.ItemNode[*appInst] // provides Root() + a stable NodeID
}

// PinInto loads children (DB rows, files, API results) onto the Pin.
func (it *notesRoot) PinInto(dst *std.Pin[*appInst]) error {
    for _, note := range loadNotes() {
        dst.AddChild(&noteItem{note: note})
    }
    return nil
}

// MarshalAttrs writes this node's attrs into the outgoing tx.
func (it *notesRoot) MarshalAttrs(w std.ItemWriter) {
    w.PutText(std.Attr.ItemLabel.ID, "My Notes")
}
```

### 2. Interactive — Bidirectional, Reactive State

For editors, viewers, anything that takes writes.  Register typed bindings with [`amp.AttrBinding[V]`](amp.support.bindings.go); incoming `TxMsg` ops dispatch to typed callbacks, and the binding caches current item state for you.

```go
notes := amp.NewAttrBinding[*NoteValue](NotesAttr) // one attr → typed items

// Per-item callback: fires for each create/update/delete in an incoming TxMsg.
notes.OnItem = func(item amp.AttrItem[*NoteValue]) {
    // ... react to item.Value (item.Deleted marks a removal)
}

// Read accumulated state back, typed:
notes.EnumItems(func(itemID tag.UID, v *NoteValue) bool {
    // ... inspect current value
    return true
})

// Author a write through the binding's node/attr context:
notes.Bind(nodeID)
notes.UpsertItem(tx, itemID, &NoteValue{ /* ... */ })
```

> The interfaces above are real (`api.app.go`, `amp/std/api.std.go`, `amp.support.bindings.go`); the bodies are illustrative.  For a complete, runnable channel that stays strictly within the public `amp` / `amp/std` API, read [`app.hello`](https://github.com/art-media-platform/amp.planet/blob/main/amp/apps/app.hello/app.hello.go) — the minimal send-only example, registered in [`amp/host/std-apps.go`](https://github.com/art-media-platform/amp.planet/blob/main/amp/host/std-apps.go).  For interactive bindings, [`app.home`](https://github.com/art-media-platform/amp.planet/blob/main/amp/apps/app.home/home.go) binds the home planet's typed item state via `AttrBinding`.

---

## Worked Example: a `notes` Channel

**Server side (Go).** A complete, registerable module is ~30 lines:

```go
package notes

import (
    "github.com/art-media-platform/amp.SDK/amp"
    "github.com/art-media-platform/amp.SDK/amp/std"
)

var AppTag = std.Attr.App.With("notes") // invocation tag: amp://{planet}/notes/...

func RegisterWith(reg amp.Registry) {
    reg.RegisterModule(&amp.AppModule{
        Info: amp.AppModuleInfo{
            Tag:     AppTag,
            Label:   "notes — a tiny channel",
            Version: "v000",
            Aliases: []string{"notes"},
        },
        NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
            app := &appInst{}
            app.Instance = app
            app.AppContext = ctx
            return app, nil
        },
    })
}

type appInst struct {
    std.AppModule[*appInst] // skeleton: MakeReady/OnClosing/PinAndServe
}

func (app *appInst) StartPin(req *amp.Request) (amp.Pin, error) {
    return app.PinAndServe(&notesRoot{}, req)
}
```

Register it in your `ampd` build where the host assembles its registry — in amp.planet that site is [`amp/host/std-apps.go`](https://github.com/art-media-platform/amp.planet/blob/main/amp/host/std-apps.go), alongside the stock apps:

```go
reg := std.Registry()   // the host's module registry
notes.RegisterWith(reg) // your one line, beside the stock apps
```

That call is the *entire* integration surface.

**Client side (web).** The [amp-web-SDK](../amp-web/) consumes the same channel by name.  No glue, no schema duplication — the wire contract is shared:

```tsx
// useAmpQuery<T>(channel, attr, opts) — 'notes' is the channel verb, 'snapshot' the attr name:
const { data, loading } = useAmpQuery<Note>('notes', 'snapshot', { limit: 50 });

// Write — one signed, sealed TxMsg (channel, attr, itemID, value):
const { upsert } = useAmpMutation();
await upsert('notes', 'snapshot', noteID, { title, body });
```

A Unity or Unreal client reaches the same channel through `libampd`; a headless agent reaches it through the Go API directly.  One channel, every surface.

---

## Handling App Pins

Clients invoke a channel by URL.  The path's first segment resolves to your `AppModule` tag (or an `Alias`); the rest is yours to interpret via `req.InvokeURL` / `req.Params`:

```
amp://{planet}/notes/inbox?since=2026-05-01
       │        │     └── your path + query (req.Params)
       │        └──────── verb → AppModule tag / alias
       └───────────────── planet UID or federated FQDN (resolved via NameService)
```

`amp://~/...` targets the caller's own home planet.

## Inherit for Free

These rails are worth standing on because when you implement a channel these are immediately yours:

| *Feature* | |
|---------|-|
| **Sovereign Secrecy** | End-to-end by construction — key exchange, epoch rotation, sealed-box wraps, every TxMsg sealed under the planet/channel epoch key.  Your keys, your crypt; the wire carries ciphertext or it carries nothing |
| **Authorship + Integrity** | Every write signed by its author; membership proven to relays without revealing identity |
| **CRDT Sync** | Offline edits, automatic conflict-free merge, multi-device convergence |
| **Transport Independence** | TCP, UDP, USB hand-carry, or [Reticulum](../README.md#reticulum) mesh over LoRa — your channel doesn't know or care |
| **Vault Infrastructure** | Stores and forwards your channel's traffic and can decrypt none of it as a zero-knowledge relay |
| **Assets** | Content-addressed, encrypted file attach — stream a 10 GB capture or a 2 KB thumbnail, deduplicated by content hash, resolved by `asset:` URI (`Session.SeedBlob` / `StoreBlob`) |
| **Federation + Naming** | Cross-planet resolution through [`Brand`](amp.core.proto) + NameService — no DNS authority, no central registry |
| **Lifecycle** | Parent-child `task.Context` close semantics; an offline outbox that drains when connectivity returns |



---

## Unity Rides the Same Rails

Nothing about the channel contract is web-specific or Go-specific — it doesn't even know what a "client" is.  So the entire AMP 3D runtime ([`amp.3D.unity`](https://github.com/art-media-platform/amp.3D.unity)) is *just another channel client*.  Scene and entity state ride CRDT + proto deltas across `(NodeID, AttrID)` cells; meshes, textures, audio, and video resolve as content-addressed **assets** by `asset:` URI.  No bespoke transport, no second sync engine — the game engine inherits end-to-end encryption, offline merge, and federation by *being a channel client*.

And Unity already models it this way, 1:1.  A `LiveCrate` is literally an `IResponder<AssetRequest>` — a scope-scoped responder that answers asset requests by address.  That is the [`Pin`](api.app.go) pattern (serve `(NodeID, AttrID)` state on request) wearing a C# hat; `CrateDepot` and `AssetCrate` are the same shape.  **A Crate is a channel; an asset is a multi-addressed cell.** 

---

## Map of This Package

| File | What's in It |
|------|--------------|
| [`api.app.go`](api.app.go) | `AppModule`, `AppInstance`, `Pin`, `Pinner`, `TxMsg`, `TxOp`, `AttrDef` — the channel contract |
| [`api.host.go`](api.host.go) | `Host`, `Session`, `Transport`, `Registry`, `Request`, asset + journal interfaces |
| [`amp.support.bindings.go`](amp.support.bindings.go) | `AttrBinding[V]`, `NodeResponder` — typed reactive state |
| [`amp.support.attrs.go`](amp.support.attrs.go) | Attr helpers and well-known attr definitions |
| [`amp.support.tx.go`](amp.support.tx.go) | `TxMsg` construction, marshaling, `Upsert` |
| [`amp.support.epoch.go`](amp.support.epoch.go) | Planet/channel epoch key plumbing |
| [`amp.core.proto`](amp.core.proto) | The wire format: every serialized type, cast in stone |
| [`std/`](std/) | `AppModule[T]`, `Item`, `Pin[T]`, `PinAndServe`, `ItemWriter` — the app-builder's toolkit |
| [`webapi/`](webapi/) | The `/api/v1/*` HTTP/JSON contract the web SDK speaks |

### Public API Surface

A third party imports only `amp`, `amp/std`, and `stdlib/*` from this SDK.  Everything a host
assembles on top — `amp.planet/amp/{vault,codex,host}` and the first-party `apps/*` — is
**internal**: useful to read, not API to depend on.  If your channel needs something only an
internal package exposes, that is a gap to raise, not an import to reach for.  The runnable
example that stays inside this line is [`app.hello`](https://github.com/art-media-platform/amp.planet/blob/main/amp/apps/app.hello/app.hello.go).

### Then Go Deeper

- [`../README.md`](../README.md) — the platform: planets, epochs, vaults, federation, the threat model
- [`../stdlib/safe/`](../stdlib/safe/README.md) — `Enclave`, `KitSpec`, keys, AEAD (all key material lives here)
- [`../stdlib/tag/`](../stdlib/tag/README.md) — UID derivation, `Address`, the addressing algebra
- [`../stdlib/task/`](../stdlib/task/api.task.go) — the goroutine lifecycle model every Pin lives in
- [`../amp-web/`](../amp-web/) — `@art-media-platform/web`, the TypeScript/React consumer; full contract in [`SKILL-amp-web-SDK.md`](../amp-web/SKILL-amp-web-SDK.md)
- [`amp.planet/docs/INDEX.md`](https://github.com/art-media-platform/amp.planet/blob/main/docs/INDEX.md) — the deep design specs (PRDs) behind this SDK: architecture, security/sync, crates, Manifold, commerce

---

<p align="center"><em>One type to implement.  A federated, encrypted, offline-first channel to ship.</em></p>
