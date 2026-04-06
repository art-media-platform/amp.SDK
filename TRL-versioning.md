# TRL Versioning

> SemVer tells you what changed.
> TRL Versioning tells you how *ready* it is.

`TRL Versioning` is a three-digit major release number expressing software maturity and readiness.  It is compatible with normal `.minor.patch` semantic versioning so you can still ship `500.1`, `500.1.1`, `501`, and so on.

---

## TRL Scale

| Band | Title | What It Means | Achievements |
|------|-------|---------------|----------------------------------|
| `000` | Conceptual | An idea with maybe some code. Could become anything. | You have a repo. |
| `100` | Alpha | Basic implementation exists.  Demonstrates value proposition. | It runs and does things related to its claims. |
| `200` | Beta (early) | Useful, testable, value is self-evident. | External people can use it and file meaningful bugs. |
| `300` | Beta (late) | Core capability maturity; architecture complete and encompassing. | Architecture survives adversarial review.  Core claims are proven under pressure, not just demos. |
| `400` | Production Ready | Core behavior validated, users see value decisively. | You'd give this to a paying customer with a straight face.  Docs, error handling, and upgrade paths exist. |
| `500` | In-Production | Real teams and workflows depend on this. | Actual humans rely on it daily.  Feedback is flowing.  Breaking changes require migration paths. |
| `600` | Scaled | Operates under large-scale real-world workloads. | Performance, observability, and ops tooling are proven — not just the features.  You have on-call stories. |
| `700` | Mature | Polished, predictable, sorted. | The API is stable.  Upgrades are boring.  New contributors can onboard without archaeology.  The codebase teaches itself. |
| `800` | Reference | Industry reference — what others measure against. | External projects depend on or cite yours.  Auditors, standards bodies, or regulatory reviews have signed off.  Runs critical infrastructure. |
| `900` | Venerable | Generational durability.  Software people forget is there because it just works. | Multi-year production across diverse environments.  The failure modes are documented because they happened and were survived.  Think SQLite, OpenSSH, zlib. |

---

## Why Use It

- It makes maturity visible at a glance.
- It rewards proof, stability, and adoption — not just shipping fast.
- It discourages fake version inflation.
- It gives teams something worth aiming for: `900` is not "old", it means the software has genuinely earned the highest level of trust.

---

## Understanding Versions

```
  002     → concept with experimental code
  236     → early beta; undergone major changes
  407     → production-ready
  503.2   → in-production, second minor revision
  701.1.2 → mature, first minor revision, second patch
```
You are supposed to run out of room within a band.

If a team burns through an entire band without moving up, that's useful signal: the product is changing, but not maturing.  That gentle pressure is part of the system — it asks you to earn the next level, not just increment a counter.
