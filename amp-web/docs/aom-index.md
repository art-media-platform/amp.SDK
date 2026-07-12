# AOM Design References — Reading Guide

The bundle's `AOM/` directory carries a curated slice of AMP's internal
Operations Manual — background reading, **not the contract** (the
contract is the README, `SKILL-amp-web-SDK.md`, and
`webapi/webapi.types.go`). The docs are written for AMP engineers, so
they reference internal chapters that are deliberately not shipped;
those references are marked `(internal)` — background provenance, not
required reading.

## Start Here

| Doc | Read it for |
|---|---|
| `AOM/DD-architecture-overview.md` | The whole-stack design tour; its §0 defines "channel" precisely (`docs/concepts-primer.md` is the web-dev digest). |
| `AOM/SD-content-substrate.md` | The data model under channels/attrs: `tag.UID`, and the `Tag` content tree behind item values. |
| `AOM/AD-app-www.md` | `app.www`, the portal your client talks to: auth flow, REST + WebSocket surface, media, cards, deep links. |

## When a Question Comes Up

| Doc | Read it when you're asking |
|---|---|
| `AOM/SD-edit-resolution.md` | "Two members edited the same item — who wins?" (CRDT concurrent-write semantics; both survive.) |
| `AOM/SD-withdrawal-consent.md` | "What does `withdraw` actually assert?" — the consent-withdrawal primitive behind `WithdrawNote` (cited by `webapi.types.go`). |
| `AOM/DD-name-service.md` | "How does an FQDN resolve to a planet, and what makes a resolve `Verified`?" (NameService, `Brand`, `TrustState` — SKILL §4.6.) |
| `AOM/SD-substrate-agnostic-members.md` | "What is `member.Kind`, really?" — identity taxonomy as `tag.UID`, never an entitlement (SKILL §12). |
| `AOM/SD-invite-governance.md` | "What are invite policies, ceilings, revocation, and the redemption ledger?" (behind `issueInvite` / `acceptInvite` — SKILL §4.7.) |
| `AOM/SD-canonization-spec.md` | "Why can't I canonize names client-side?" — the byte-level rules behind `resolveTag` / forge (SKILL §5.8). |
| `AOM/AD-app-forums.md` | "How does a real app structure channels, verbs, and a custodian?" — the design behind `examples/forums/`. |

## What Is Deliberately Not Shipped

Operator runbooks, security-cluster internals, and audit files. If a
shipped doc cites one and the answer matters to your integration, ask
your amp contact rather than guessing.
