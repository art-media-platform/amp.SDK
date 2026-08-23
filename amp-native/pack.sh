#!/usr/bin/env bash
# Build the amp-native-SDK distributable kit (the native rail; free tier).
#
#   amp.SDK/amp-native/pack.sh v0.NNN.N
#     →  amp.SDK/dist/amp-native-SDK-{tier}-v0.NNN.N-{platform}.zip
#
# Pre-release version identity: amp.planet carries no release tags, so the
# version is the REQUIRED argument (the release-record version), and the kit
# carries a VERSION build record (UTC time, kit name, binary SHA-256, source
# commits).  Platform is the build host's go env (darwin-arm64 first).
#
# Inputs: the sibling amp.planet checkout (CLI source + private AOM) and this
# repo.  AOM shipping is an explicit allowlist with the same tooling as
# amp-web-SDK (pack-aom-public.mjs marker subset + pack-delink.mjs link gate —
# one authoritative site, reused from ../amp-web).  A listed doc that is
# missing FAILS the build — never warn-and-skip.
#
# Env overrides:
#   AMP_NATIVE_TIER        tier token in the artifact name (default: free)
#   AMP_PLANET_SRC         amp.planet checkout (default: ../amp.planet)
#   AMP_NATIVE_AOM_SRC     AOM dir (default: $AMP_PLANET_SRC/AOM)
#
# NOTE: the amp build runs in the caller's Go environment.  Building from a
# worktree outside go.work needs GOWORK=off and the dev SDK bridge
# (`make sdk-dev` in the amp.planet checkout) per O4 §4.14.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"   # amp.SDK/amp-native
SDK="$(cd "$HERE/.." && pwd)"                           # amp.SDK

VER="${1:?usage: pack.sh v0.NNN.N   (the release-record version; amp.planet is untagged pre-release)}"
case "$VER" in
  v0.*.*) ;;
  *) echo "ERROR: version must match v0.NNN.N (got: $VER)" >&2; exit 2 ;;
esac

TIER="${AMP_NATIVE_TIER:-free}"
PLANET="${AMP_PLANET_SRC:-$SDK/../amp.planet}"
AOM_SRC="${AMP_NATIVE_AOM_SRC:-$PLANET/AOM}"
PLATFORM="$(go env GOOS)-$(go env GOARCH)"

KIT="amp-native-SDK-$TIER-$VER-$PLATFORM"
STAGE="$SDK/build/amp-native-SDK"                       # extracts to ./amp-native-SDK
OUT="$SDK/dist"
ZIP="$OUT/$KIT.zip"

if [ ! -d "$PLANET/cmd/amp" ]; then
  echo "ERROR: amp CLI source not found at $PLANET/cmd/amp (set AMP_PLANET_SRC)" >&2
  exit 1
fi
if [ ! -d "$AOM_SRC" ]; then
  echo "ERROR: AOM not found at $AOM_SRC — the kit ships the AOM shelves (set AMP_NATIVE_AOM_SRC)" >&2
  exit 1
fi

echo "+++ $KIT"

# 1. Build the amp CLI for the host platform.
rm -rf "$STAGE"
mkdir -p "$STAGE/bin"
( cd "$PLANET/cmd/amp" && touch main.go && go build -o "$STAGE/bin/amp" . )

# 2. Stage the entry docs + license.  The web-rail SKILL doc ships with its
#    own support docs (its relative links must resolve in-bundle; delink gates).
cp "$HERE/README.md" "$STAGE/README.md"
cp "$SDK/LICENSE" "$STAGE/LICENSE"
cp "$SDK/amp-web/SKILL-amp-web-SDK.md" "$STAGE/SKILL-amp-web-SDK.md"
cp "$SDK/amp-web/SECURITY-amp-web-SDK.md" "$STAGE/SECURITY-amp-web-SDK.md"
mkdir -p "$STAGE/docs"
for doc in get-a-backend.md install-troubleshooting.md siwe-primer.md aom-index.md; do
  cp "$SDK/amp-web/docs/$doc" "$STAGE/docs/$doc"
done

# 3. Curated AOM shelves — explicit allowlist, never cp -R.
#    AOM_DOCS ship whole; AOM_PUBLIC_SUBSET are operator chapters that ship
#    only their `[PUBLIC]`-marked sections (pack-aom-public.mjs).
AOM_DOCS=(
  DD-name-service.md
)
AOM_PUBLIC_SUBSET=(
  O4-standard-procedures.md
)
mkdir -p "$STAGE/AOM"
for doc in "${AOM_DOCS[@]}"; do
  if [ ! -f "$AOM_SRC/$doc" ]; then
    echo "ERROR: allowlisted AOM doc missing: $AOM_SRC/$doc" >&2
    exit 1
  fi
  cp "$AOM_SRC/$doc" "$STAGE/AOM/$doc"
done
for doc in "${AOM_PUBLIC_SUBSET[@]}"; do
  if [ ! -f "$AOM_SRC/$doc" ]; then
    echo "ERROR: allowlisted AOM chapter missing: $AOM_SRC/$doc" >&2
    exit 1
  fi
  node "$SDK/amp-web/pack-aom-public.mjs" "$AOM_SRC/$doc" "$STAGE/AOM/$doc"
done

# 4. De-link pass: every relative link in every staged .md resolves within the
#    bundle, or is rewritten to the `(internal)` token; dangling links FAIL.
node "$SDK/amp-web/pack-delink.mjs" "$STAGE" "$AOM_SRC"

# 5. VERSION build record (one line, deploy-log shape: time, name, hash,
#    source commits).
BIN_SHA="$(shasum -a 256 "$STAGE/bin/amp" | cut -d' ' -f1)"
PLANET_REV="$(git -C "$PLANET" rev-parse --short HEAD)"
SDK_REV="$(git -C "$SDK" rev-parse --short HEAD)"
printf '%s  %s  %s  amp.planet@%s amp.SDK@%s\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$KIT" "$BIN_SHA" "$PLANET_REV" "$SDK_REV" \
  > "$STAGE/VERSION"

# 6. Strip cruft; zip from the build root so the archive contains a single
#    amp-native-SDK/ dir.
find "$STAGE" -name '.DS_Store' -delete 2>/dev/null || true
mkdir -p "$OUT"
rm -f "$ZIP"
( cd "$SDK/build" && zip -rq "$ZIP" amp-native-SDK )
rm -rf "$SDK/build"

echo "+++ wrote $ZIP ($(du -h "$ZIP" | cut -f1))"
unzip -l "$ZIP"
