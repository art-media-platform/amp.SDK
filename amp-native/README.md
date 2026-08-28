# amp-native-SDK

The native rail of AMP's two partner kits. Its sibling, **amp-web-SDK**, is the
web rail — a pure JS/TS package that never carries a native binary. This kit
carries the native surface: the `amp` CLI and the `ampd` node daemon, plus the
curated AMP Operations Manual (AOM) shelves a CLI partner works from.

Use of this kit is governed by the bundled `LICENSE` (Beta Evaluation
License) — evaluation and interoperating-app use, AS-IS, no redistribution of
the kit itself. `bin/ampd` carries one further limit: development and
evaluation only, production operation not licensed (`LICENSE` §3).

References marked (internal) name AMP-internal design docs not shipped in this bundle — background provenance, not required reading.

## Contents

| Path | What It Is |
|---|---|
| `bin/amp` | the `amp` CLI, built for the platform in the kit filename |
| `bin/ampd` | the `ampd` node daemon — development and evaluation use only (see Running a Node Locally) |
| `AOM/O4-standard-procedures.md` | the partner subset of AMP's standard procedures — retrieve/embed/redeem an invite (§4.4–§4.6), publish a name (§4.10), bring up a local follower node (§4.17), the mint-and-connect walk (§4.18) |
| `AOM/DD-name-service.md` | the naming and trust design: how planets are named, branded, resolved, and verified |
| `docs/amp-cli.md` | the `amp` command reference, generated from the binary in this kit |
| `docs/get-a-backend.md` | how to reach a node: your operator's, or the one in `bin/` |
| `docs/install-troubleshooting.md` | what to try when a step above does not work |
| `docs/siwe-primer.md` | signing in with a wallet, for the web rail |
| `docs/aom-index.md` | what each shipped AOM shelf covers |
| `SKILL-amp-web-SDK.md` | the web-rail skill doc — drop it into a web project to build against `app.www`; the web kit itself ships separately as amp-web-SDK |
| `SECURITY-amp-web-SDK.md` | the web rail's security model: what the client may hold, and what it may never |
| `LICENSE` | the Beta Evaluation License this kit is delivered under |
| `VERSION` | the build record for this exact kit (see Version Identity) |

## Quick Start

1. Put `bin/amp` on your `PATH`. On macOS, open the disk image first and
   copy `bin/` wherever you keep tools — the image is a container, not an
   installer, so where the binaries live is your choice.
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

No reachable node yet? `bin/ampd` is one — §4.17 brings up a local follower
node for client dogfooding. No AMP-operated eval planet endpoint is named in
this kit revision; first contact runs against your operator's node, or against
the node you run yourself.

## Running a Node Locally

`bin/ampd` is the AMP node daemon: the server software an operator runs, the
thing `bin/amp` and an amp-web-SDK app both talk to. It ships here so the kit
is self-contained — you can work the whole ladder (found, invite, redeem,
brand, publish, resolve) on your own machine without waiting on someone else's
endpoint.

What it is for in this kit:

- the local follower node of `AOM/O4-standard-procedures.md` §4.17 — a real
  peer for the CLI and for a web app under development;
- serving your own AppModule against a node you control while you build it;
- running the §4.18 mint-and-connect walk end to end.

`ampd -h` prints the full flag set. §4.17 gives the working invocation and the
one trap worth knowing up front: `-port.host` does not move the HTTP port, so
set `-port.http` too, or a second local node collides on the default. Point
`-path.persist` and `-path.cache` at directories reserved for the development
node.

**Production operation is not licensed.** Serving real users or real data —
any live, public, or revenue-bearing service, internal or external — needs a
commercial license (`LICENSE` §3), as does `libampd`, the embedding library,
which this kit does not carry. The line is simple: clients are free, running
a node for others and embedding AMP in a shipped product are not.

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

Ask the binary first:

```
bin/amp version
```

```
amp v0.279.0
  amp.planet  72b53af
  amp.SDK     3c48e83
  toolchain   go1.26.3  darwin/arm64
```

Two commits, because AMP's node and its SDK are separate sources — either one
alone does not identify a build. `bin/ampd -version` prints the same record. A
field the build could not resolve reads `ERR-BuildInfo`; that is a defect worth
reporting, never a version.

`VERSION` carries the same identity for the kit as a whole: the kit's full
name, the SHA-256 of each shipped binary, and the source commits. The kit
filename carries the rest:

```
amp-native-SDK-{tier}-v0.NNN.N-{platform}.dmg    macOS
amp-native-SDK-{tier}-v0.NNN.N-{platform}.zip    Linux, Windows
```

macOS ships a disk image because a notarization ticket can only be stapled into
a container. The ticket then travels with the download, which is what lets the
kit verify — and run — with no network.

Not on a Mac and need to look inside the macOS kit? `7z x <kit>.dmg` unpacks a
disk image on Linux and Windows. That is extraction only — the notarization
checks below are macOS-only.

## Checking What You Downloaded

The kit ships alongside `checksums.txt` and `verify-release-binaries.sh` —
together they are the whole attestation story. Put them beside the downloads and
run the script:

```
./verify-release-binaries.sh
```

| What it establishes | Where |
|---|---|
| every download matches its recorded SHA-256 | everywhere |
| the disk image is signed, notarized and stapled | macOS |
| a binary you copied out is signed, and its ticket is readable | macOS |

The macOS checks need no network. The ticket is stapled into the image, so once
you have opened the image both it and the binaries you copy out of it verify
offline.

`checksums.txt` carries no detached signature; the digests and the macOS
notarization are what this kit attests. The script says so on every run rather
than implying a signature it does not have — read its `!!!` lines, which name
what it could NOT check rather than passing silently.

**Windows binaries are not code-signed.** SmartScreen will warn on first run;
`checksums.txt` is how you confirm what you downloaded.

## SDK Access Tiers

This is the **Free — Evaluation** kit: the client surface, plus a node you may
run for development and evaluation. Higher SDK access tiers add the operator
and embedding surfaces — production `ampd` operation, `libampd`, and the
whitelabel authoring slice — under commercial license. The kit name never
changes across tiers; contents and license do. Ask your AMP contact about
Creator and Whitelabel/OEM licensing.

Contact: licensing@art.media.platform
