#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUT="$ROOT/dist"
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

rm -rf "$OUT"
mkdir -p "$OUT" "$STAGE/bin"
cp -a "$ROOT/module/." "$STAGE/"

CGO_ENABLED=0 GOOS=android GOARCH=arm64 \
  go build -trimpath -ldflags='-s -w' \
  -o "$STAGE/bin/limbus-proxy" "$ROOT/cmd/limbus-proxy"

chmod 0755 "$STAGE/bin/limbus-proxy" "$STAGE"/*.sh
chmod 0644 "$STAGE/module.prop"

if [[ -n "${MODULE_VERSION:-}" ]]; then
  sed -i "s/^version=.*/version=${MODULE_VERSION}/" "$STAGE/module.prop"
fi
if [[ -n "${MODULE_VERSION_CODE:-}" ]]; then
  sed -i "s/^versionCode=.*/versionCode=${MODULE_VERSION_CODE}/" "$STAGE/module.prop"
fi

(cd "$STAGE" && zip -q -r "$OUT/limbus-localization-ksu.zip" .)
(cd "$OUT" && sha256sum limbus-localization-ksu.zip > checksums.sha256)
echo "Built $OUT/limbus-localization-ksu.zip"
