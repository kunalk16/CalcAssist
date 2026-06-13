#!/usr/bin/env bash
# Build calcassist (Linux/macOS). Produces a static, CGO-free binary.
#
# Usage:
#   ./build.sh                 # build for the host OS into ./bin
#   ./build.sh all             # cross-compile windows/linux/darwin (amd64+arm64) into ./dist
#   VERSION=1.2.3 ./build.sh   # stamp an explicit version
set -euo pipefail

VERSION="${VERSION:-0.1.0}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
DATE="$(date +%Y-%m-%d)"
PKG="calcassist/internal/version"
MAIN="./cmd/calcassist"
LDFLAGS="-s -w -X '${PKG}.Version=${VERSION}' -X '${PKG}.Commit=${COMMIT}' -X '${PKG}.Date=${DATE}'"

export CGO_ENABLED=0

build() {
  local goos="$1" goarch="$2" outdir="$3"
  local ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  local out="${outdir}/calcassist-${goos}-${goarch}${ext}"
  echo "Building ${goos}/${goarch} -> ${out}"
  GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" -o "$out" "$MAIN"
}

if [ "${1:-}" = "all" ]; then
  mkdir -p dist
  build windows amd64 dist
  build windows arm64 dist
  build linux   amd64 dist
  build linux   arm64 dist
  build darwin  amd64 dist
  build darwin  arm64 dist
  echo "Done. Artifacts in ./dist"
else
  mkdir -p bin
  echo "Building host -> bin/calcassist"
  go build -trimpath -ldflags "$LDFLAGS" -o bin/calcassist "$MAIN"
  echo "Done. Binary at bin/calcassist"
fi
