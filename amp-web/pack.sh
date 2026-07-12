#!/usr/bin/env bash
# Build the amp-web-SDK distributable bundle.
#
#   amp.SDK/amp-web/pack.sh   →   amp.SDK/dist/amp-web-SDK-<vNNN>.zip
#
# Self-contained: assembles only from public amp.SDK content — the amp-web/
# TypeScript client plus the canonical wire contract in amp/webapi.  No
# amp.planet (private) inputs.  Version is derived from the amp.SDK git tag at
# build time (resolution order below), so there is one source of truth for the
# SDK revision — the committed package.json carries only a placeholder.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"   # amp.SDK/amp-web
SDK="$(cd "$HERE/.." && pwd)"                           # amp.SDK

# The amp.SDK git tag is the single source of truth for the SDK revision, so the
# bundle version is derived at build time — no package.json bump per release tag.
# Resolution order: explicit override (CI passes the tag) → the tag at HEAD →
# nearest tag (dev build, warned) → the package.json placeholder (untagged tree).
if [ -n "${AMP_WEB_VERSION:-}" ]; then
  VER="${AMP_WEB_VERSION#v}"
elif TAG="$(git -C "$SDK" describe --tags --match 'v*' --exact-match 2>/dev/null)"; then
  VER="${TAG#v}"
elif TAG="$(git -C "$SDK" describe --tags --match 'v*' --abbrev=0 2>/dev/null)"; then
  echo "!!! HEAD is not a release tag; bundling as nearest tag $TAG"
  VER="${TAG#v}"
else
  VER="$(node -p "require('$HERE/package.json').version")"
fi
# Version label: vMINOR for a .0 patch, vMINOR.PATCH otherwise.
#   0.232.0 -> v232    0.233.0 -> v233    0.233.1 -> v233.1
MINOR="$(printf '%s' "$VER" | cut -d. -f2)"
PATCH="$(printf '%s' "$VER" | cut -d. -f3)"
if [ "$PATCH" = "0" ]; then
  LABEL="v$MINOR"
else
  LABEL="v$MINOR.$PATCH"
fi
STAGE="$SDK/build/amp-web-SDK"                          # extracts to ./amp-web-SDK
OUT="$SDK/dist"
ZIP="$OUT/amp-web-SDK-$LABEL.zip"

echo "+++ amp-web-SDK $LABEL  (package version $VER)"

# 1. Build the TypeScript client.
cd "$HERE"
if [ ! -d node_modules ]; then
  npm ci --prefer-offline --no-audit --no-fund || npm install --no-audit --no-fund
fi
npm run build

# 2. Stage the bundle.
rm -rf "$STAGE"
mkdir -p "$STAGE/webapi"
cp README.md SKILL-amp-web-SDK.md SECURITY-amp-web-SDK.md LICENSE \
   package.json package-lock.json "$STAGE/"
# Stamp the resolved version into the bundled package.json (the committed source
# carries a placeholder; the tag is authoritative).  Format-preserving rewrite.
node -e 'const fs=require("fs"),p=process.argv[1],v=process.argv[2];
  fs.writeFileSync(p, fs.readFileSync(p,"utf8").replace(/("version":\s*)"[^"]*"/, `$1"${v}"`));' \
  "$STAGE/package.json" "$VER"
cp -R src     "$STAGE/src"
cp -R dist    "$STAGE/dist"
cp -R scripts "$STAGE/scripts"
cp -R docs    "$STAGE/docs"

# 3. Copy the canonical wire contract (the public source of truth) plus its
#    golden fixtures, so the bundle's own drift guard (`npm test`,
#    src/drift.test.ts) runs from the bundle view.
cp "$SDK/amp/webapi/webapi.types.go" "$STAGE/webapi/webapi.types.go"
cp -R "$SDK/amp/webapi/testdata" "$STAGE/webapi/testdata"

# 3a. Ship the example app (the reference consumer).  Its file:../.. dependency
#     resolves to the bundle root in this layout, same as in the repo.
mkdir -p "$STAGE/examples"
cp -R examples/forums "$STAGE/examples/forums"
rm -rf "$STAGE/examples/forums/node_modules" \
       "$STAGE/examples/forums/dist" \
       "$STAGE/examples/forums/.env.local"

# 3b. Bundle a curated set of AOM design docs (partner reference; see README
#     "Design References").  These live in the PRIVATE amp.planet/AOM, so this is a
#     reviewed carve-out of the public-inputs-only rule: an explicit allowlist (never
#     cp -R, so operator/audit docs can't leak).  A partner bundle is deterministic:
#     a listed doc that is missing FAILS the build — never warn-and-skip.  A
#     public/CI build without amp.planet opts out explicitly with AMP_WEB_NO_AOM=1.
AOM_SRC="$SDK/../amp.planet/AOM"
AOM_DOCS=(
  DD-architecture-overview.md
  DD-name-service.md
  SD-content-substrate.md
  SD-edit-resolution.md
  SD-withdrawal-consent.md
  SD-substrate-agnostic-members.md
  SD-canonization-spec.md
  SD-invite-governance.md
  AD-app-www.md
  AD-app-forums.md
)
if [ -n "${AMP_WEB_NO_AOM:-}" ]; then
  echo "!!! AMP_WEB_NO_AOM set — bundling without AOM design refs"
elif [ ! -d "$AOM_SRC" ]; then
  echo "ERROR: amp.planet/AOM not found at $AOM_SRC — a partner bundle ships the AOM refs." >&2
  echo "       Set AMP_WEB_NO_AOM=1 to build without them (public/CI build)." >&2
  exit 1
else
  mkdir -p "$STAGE/AOM"
  for doc in "${AOM_DOCS[@]}"; do
    if [ ! -f "$AOM_SRC/$doc" ]; then
      echo "ERROR: allowlisted AOM doc missing: $AOM_SRC/$doc" >&2
      exit 1
    fi
    cp "$AOM_SRC/$doc" "$STAGE/AOM/$doc"
  done
fi

# 3c. De-link pass: every relative link in every staged .md must resolve
#     within the bundle.  A link to an AOM doc outside the allowlist is
#     rewritten to the greppable `Text (internal)` token; any other dangling
#     link (or an instruction-class line citing an unshipped doc) FAILS the
#     pack — never warn-and-skip.  See README "Authoring Notes".
node "$HERE/pack-delink.mjs" "$STAGE" "$AOM_SRC"

# 4. Strip cruft.
find "$STAGE" -name '.DS_Store' -delete 2>/dev/null || true

# 5. Zip from the build root so the archive contains a single amp-web-SDK/ dir.
mkdir -p "$OUT"
rm -f "$ZIP"
( cd "$SDK/build" && zip -rq "$ZIP" amp-web-SDK )
rm -rf "$SDK/build"

echo "+++ wrote $ZIP ($(du -h "$ZIP" | cut -f1))"
unzip -l "$ZIP"
