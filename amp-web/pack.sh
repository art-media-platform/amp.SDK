#!/usr/bin/env bash
# Build the amp-web-SDK distributable bundle.
#
#   amp.SDK/amp-web/pack.sh   →   amp.SDK/dist/amp-web-SDK-<vNNN>.zip
#
# Self-contained: assembles only from public amp.SDK content — the amp-web/
# TypeScript client plus the canonical wire contract in amp/webapi.  No
# amp.planet (private) inputs.  Version is stamped from amp-web/package.json,
# so there is one source of truth for the SDK revision.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"   # amp.SDK/amp-web
SDK="$(cd "$HERE/.." && pwd)"                           # amp.SDK
VER="$(node -p "require('$HERE/package.json').version")"
LABEL="v$(printf '%s' "$VER" | cut -d. -f2)"            # 0.232.0 -> v232
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
mkdir -p "$STAGE/wire"
cp README.md SKILL-amp-web-SDK.md SECURITY-amp-web-SDK.md LICENSE \
   package.json package-lock.json tsconfig.json "$STAGE/"
cp -R src     "$STAGE/src"
cp -R dist    "$STAGE/dist"
cp -R scripts "$STAGE/scripts"

# 3. Copy the canonical wire contract (the public source of truth).
cp "$SDK/amp/webapi/webapi.types.go" "$STAGE/wire/webapi.types.go"

# 4. Strip cruft.
find "$STAGE" -name '.DS_Store' -delete 2>/dev/null || true

# 5. Zip from the build root so the archive contains a single amp-web-SDK/ dir.
mkdir -p "$OUT"
rm -f "$ZIP"
( cd "$SDK/build" && zip -rq "$ZIP" amp-web-SDK )
rm -rf "$SDK/build"

echo "+++ wrote $ZIP ($(du -h "$ZIP" | cut -f1))"
unzip -l "$ZIP"
