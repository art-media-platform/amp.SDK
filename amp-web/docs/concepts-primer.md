# AMP Concepts for Web Developers

The five-minute mental model behind the SDK's `(channel, attr, itemID)`
parameters. Deeper background: `AOM/DD-architecture-overview.md` (its §0
is the source of the channel definition below) and
`AOM/SD-content-substrate.md`.

## The Containment Chain

```
planet  →  node  →  attr  →  item  →  edit
```

- **Planet** — a self-governing community: its own members, encryption
  keys, and governance. Your client binds one default planet
  (`planetTag`); a member can belong to many.
- **Node** — an addressable anchor inside a planet, identified by a
  `tag.UID`. The SDK calls this parameter `channel`.
- **Attr** — a well-known attribute a node bears, also a `tag.UID`.
  The attr names the *behavior contract*: what the items in this cell
  mean and how clients should treat them.
- **Item** — one CRDT record in that `(node, attr)` cell, identified by
  `tag.UID`. What `query` returns and `tx` writes.
- **Edit** — a versioned, signed update to an item. Concurrent edits
  merge deterministically (`AOM/SD-edit-resolution.md`); history is
  retained, not overwritten.

## "Channel" Precisely: Nodes as Channels

"Channel" is convenient slang for a load-bearing addressing pattern:

```
channel ≈ (NodeID, AttrID) + behavior contract
```

Consequences worth internalizing:

- **A channel is per-attr, not per-node.** One node carries many attrs —
  many channels sharing one NodeID. Subscriptions bind a specific
  `(channel, attr)` cell, never "the node" wholesale.
- **Any UID can be the node.** Channel names like `'projects'` resolve
  to UIDs (SKILL §5.8) — but an item's own UID works too. The forums
  example posts replies to `Channel: topicID`: each topic item is
  simultaneously the *node* under which its posts live. That is
  nodes-as-channels, and it is the idiomatic way to nest data.
- **The address is the query.** There is no server-side filter or
  orderBy — you scope reads by *writing* to a scoped address
  (`widgets/instance.{memberID}`), and page by the server-enforced
  ItemID window (SKILL §5.2, "Address, don't filter").

## What the SDK Calls Sees

| You write | Substrate meaning |
|---|---|
| `query('projects', 'labels')` | read items in cell (node `projects`, attr `labels`) |
| `create('projects', 'labels', v)` | one-op TxMsg: signed, encrypted, journaled |
| `tx([...])` | N ops, one atomic TxMsg, one signature |
| `invoke('amp://~/forums/post', ops)` | hand ops to an app verb; it writes custodially (SKILL §4.3) |
| `subscribe('projects', 'labels', cb)` | live edits on exactly that cell |

Every write is an **edit** by an authenticated **member**, sealed under
the planet's current epoch key, relayed by vaults that cannot read it
(`SECURITY-amp-web-SDK.md`). Your web app is a disposable UI over that
durable substrate.
