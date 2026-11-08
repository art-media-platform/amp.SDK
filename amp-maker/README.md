# AMP Maker Kit

The native rail of AMP's two partner kits. Its sibling, the **AMP Web Kit**, is the
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
| `AOM/O4-standard-procedures.md` | the partner subset of AMP's standard procedures — retrieve and embed an invite, join a planet by invite (§4.4–§4.6), publish a name (§4.10), bring up a local replica node (§4.17), the mint-and-connect walk (§4.18) |
| `AOM/DD-name-service.md` | the naming and trust design: how planets are named, branded, resolved, and verified |
| `docs/amp-cli.md` | the `amp` command reference, generated from the binary in this kit |
| `docs/get-a-backend.md` | how to reach a node: your node operator's, or the one in `bin/` |
| `docs/install-troubleshooting.md` | what to try when a step above does not work |
| `docs/siwe-primer.md` | signing in with a wallet, for the web rail |
| `docs/aom-index.md` | what each shipped AOM shelf covers |
| `SKILL-amp-web-kit.md` | the web-rail skill doc — drop it into a web project to build against `app.www`; the AMP Web Kit itself ships separately as `amp-web-kit-vNNN[.P].zip` |
| `SECURITY-amp-web-kit.md` | the web rail's security model: what the client may hold, and what it may never |
| `LICENSE` | the Beta Evaluation License this kit is delivered under |
| `VERSION` | the build record for this exact kit (see Version Identity) |

On macOS the kit is an installer package: it places `bin/*` in
`/usr/local/bin` and everything else in `/usr/local/share/amp-maker-kit/`.

## Quick Start

1. Put `amp` on your `PATH`.
   - **macOS** (Apple silicon or Intel): open
     `amp-maker-kit-v0.NNN.N-darwin-universal.pkg`, or run
     `sudo installer -pkg <file> -target /`. A later package replaces the
     installed kit in place. The package declares its minimum macOS
     ([Deploying to a Mac Fleet](#deploying-to-a-mac-fleet)).
   - **Linux, Windows:** unzip the kit and put its `bin/` on your `PATH`.
2. Get an invite URL and its passphrase from a founder or admin of the planet (out-of-band).
3. Join with the invite — the full procedure, verification, and recovery steps are
   `AOM/O4-standard-procedures.md` §4.6:

   ```
   amp --addr {your-node}:{port} --tome member.tome \
       invite accept --file <invite-file> --invite-pass <passphrase>
   ```

   Your node operator supplies the node address; this document writes it
   `{your-node}`.
4. Confirm membership by publishing a name (§4.10). The end-to-end ladder —
   found, invite, join, brand, publish, resolve — is §4.18.

No reachable node yet? `bin/ampd` is one — §4.17 brings up a local replica
node for client dogfooding. No AMP-operated eval planet endpoint is named in
this kit revision; first contact runs against your node operator's node, or against
the node you run yourself.

## Running a Node Locally

`bin/ampd` is the AMP node daemon: the server software a node operator runs, the
thing `bin/amp` and an AMP Web Kit app both talk to. It ships here so the kit
is self-contained — you can work the whole ladder (found, invite, join,
brand, publish, resolve) on your own machine without waiting on someone else's
endpoint.

What it is for in this kit:

- the local replica node of `AOM/O4-standard-procedures.md` §4.17 — a real
  peer for the CLI and for a web app under development;
- running the §4.18 mint-and-connect walk end to end.

It runs AMP's built-in apps only. A Go AppModule of your own is compiled into
a host build that registers it; this `ampd` cannot load one.

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

- The issuer chooses each invite's passphrase; hosts refuse one below the
  floor from **any** client: at least **10 characters**, 2 or more of them not
  letters (a digit, a symbol, punctuation).
- Spaces and capitals are not counted and do not matter: `Blue Moon, 42
  tacos!` and `bluemoon,42tacos!` open the same invite.
- `invite issue` and `federation found --invite` take `--invite-pass`;
  `invite issue --count N` (N > 1) generates a distinct pass per invite
  instead. `invite accept` and `invite inspect` require `--invite-pass`.

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
amp-maker-kit-v0.NNN.N-darwin-universal.pkg     macOS
amp-maker-kit-v0.NNN.N-{platform}.zip           Linux, Windows
```

macOS ships a signed, notarized installer package. Its notarization ticket is
stapled in, so it verifies with no network, and the files it places carry no
download quarantine attribute: `amp` and `ampd` run from a launchd service,
cron or `ssh` with no first-run prompt. Every document in the package also
ships in the Linux and Windows zips.

## Deploying to a Mac Fleet

The macOS package is built for fleet tools (Jamf, Kandji, Intune): a flat
distribution package signed with a Developer ID Installer certificate,
notarized and stapled, with a payload and no install scripts. Upload it
unchanged.

| What a fleet tool asks for | Answer |
|---|---|
| install command | `installer -pkg <file> -target /` (system domain only) |
| receipt identifier | `com.art-media-platform.amp-native-SDK` |
| version | `0.NNN.N`, rising with every release |
| detection | `pkgutil --pkg-info com.art-media-platform.amp-native-SDK` exits 0 and reports the version |
| minimum macOS | the one the package declares: `pkgutil --expand <file> <dir>`, then `grep os-version <dir>/Distribution`; the package refuses an older system |
| architecture | one universal package for Apple silicon and Intel |

The package installs no `.app`, so a tool that detects installs by app bundle
ID (Intune's Included apps) has nothing to match: detect it by the receipt
instead, with a script or extension attribute.

To remove the kit:

```
sudo rm /usr/local/bin/amp /usr/local/bin/ampd
sudo rm -r /usr/local/share/amp-maker-kit
sudo pkgutil --forget com.art-media-platform.amp-native-SDK
```

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
| the package is signed, notarized and stapled | macOS |

The macOS check needs no network: the ticket is stapled into the package.

`checksums.txt` carries no detached signature; the digests and the macOS
notarization are what this kit attests. The script says so on every run rather
than implying a signature it does not have — read its `!!!` lines, which name
what it could NOT check rather than passing silently.

**Windows binaries are not code-signed.** SmartScreen will warn on first run;
`checksums.txt` is how you confirm what you downloaded.

## SDK Access Tiers

This kit is the same at every SDK access tier. Delivered under the bundled
Beta Evaluation License, it covers the client surface plus a node you may run
for development and evaluation. Higher tiers change the license, not the kit:
production `ampd` operation is a commercial grant over these same binaries,
and `libampd` and the brand-mint authoring slice arrive as separate packages.
Ask your AMP contact about Creator and Brand Mint licensing.

Contact: licensing@art.media.platform
