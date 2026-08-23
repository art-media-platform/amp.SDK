# amp-native-SDK

The native rail of AMP's two partner kits. Its sibling, **amp-web-SDK**, is the
web rail — a pure JS/TS package that never carries a native binary. This kit
carries the native surface: the `amp` CLI plus the curated AMP Operations
Manual (AOM) shelves a CLI partner works from.

Use of this kit is governed by the bundled `LICENSE` (Beta Evaluation
License) — evaluation and interoperating-app use, AS-IS, no redistribution of
the kit itself.

References marked (internal) name AMP-internal design docs not shipped in this bundle — background provenance, not required reading.

## Contents

| Path | What It Is |
|---|---|
| `bin/amp` | the `amp` CLI, built for the platform in the kit filename |
| `AOM/O4-standard-procedures.md` | the partner subset of AMP's standard procedures — retrieve/embed/redeem an invite (§4.4–§4.6), publish a name (§4.10), bring up a local follower node (§4.17), the mint-and-connect walk (§4.18) |
| `AOM/DD-name-service.md` | the naming and trust design: how planets are named, branded, resolved, and verified |
| `SKILL-amp-web-SDK.md` | the web-rail skill doc — drop it into a web project to build against `app.www`; the web kit itself ships separately as amp-web-SDK |
| `LICENSE` | the Beta Evaluation License this kit is delivered under |
| `VERSION` | the build record for this exact kit (see Version Identity) |

## Quick Start

1. Put `bin/amp` on your `PATH`.
2. Get an invite URL and its passphrase from your operator (out-of-band).
3. Redeem it — the full procedure, verification, and recovery steps are
   `AOM/O4-standard-procedures.md` §4.6:

   ```
   amp --addr {your-node}:{port} --tome member.tome \
       invite accept --file <invite-file> --invite-pass <passphrase>
   ```

   Your operator supplies the node address; this document writes it
   `{your-node}`.
4. Confirm membership by publishing a name (§4.10). The end-to-end ladder —
   found, invite, redeem, brand, publish, resolve — is §4.18.

No reachable node yet? §4.17 brings up a local follower node for client
dogfooding. No AMP-operated eval planet endpoint is named in this kit
revision — first contact runs against your operator's node.

## Invite Passphrase Floor

Ships with this kit as the compatibility notice for existing CLI holders:

- Hosts now refuse a sub-floor invite sealing passphrase from **any**
  client: at least **6 words** (8 for grants). Short or default passphrases
  (`test123`-class) are refused host-side at mint.
- This CLI requires `--invite-pass` explicitly on `invite accept` and
  `invite inspect` — there is no typed default.
- Older CLI builds still work against current hosts **only** when the
  operator supplies a typed passphrase meeting the floor. This kit is the
  upgrade path.

## The Shelves

The `AOM/` docs are the partner subset of AMP's Operations Manual.
Coordinates marked `(operator)` name procedures held in AMP's Operations
Manual and not shipped in this bundle. Cross-references marked `(internal)`
follow the legend above.

## Version Identity

AMP is pre-release; the kit is versioned by its build record, not a source
tag. `VERSION` carries one line: build time (UTC), the kit's full name, the
SHA-256 of `bin/amp`, and the source commits it was built from. The kit
filename carries the rest of the identity:

```
amp-native-SDK-{tier}-v0.NNN.N-{platform}.zip
```

## SDK Access Tiers

This is the **Free — Evaluation** kit: the client surface. Higher SDK access
tiers add the operator and embedding surfaces (production `ampd`, `libampd`,
and the whitelabel authoring slice) under commercial license. The kit name
never changes across tiers — contents and license do. Ask your AMP contact
about Creator and Whitelabel/OEM licensing.

Contact: licensing@art.media.platform
