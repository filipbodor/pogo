#!/bin/sh
set -eu

version=${1:?Usage: ./scripts/release.sh vMAJOR.MINOR.PATCH}
case "$version" in
  *[!a-zA-Z0-9.+-]*|'') echo "Invalid version" >&2; exit 1 ;;
esac
case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "Version must start with vMAJOR.MINOR.PATCH" >&2; exit 1 ;;
esac

# Resolve the checkout independently of the caller's working directory.
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
mkdir -p dist
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT HUP INT TERM

for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do
  target_os=${target%/*}
  target_arch=${target#*/}
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath \
    -ldflags "-s -w -X main.version=${version#v}" -o "$stage/pogo" ./cmd/pogo
  cp README.md LICENSE "$stage/"
  tar -czf "dist/pogo_${version}_${target_os}_${target_arch}.tar.gz" -C "$stage" \
    pogo README.md LICENSE
done

if command -v shasum >/dev/null 2>&1; then
  (cd dist && shasum -a 256 "pogo_${version}_"*.tar.gz > checksums.txt)
else
  (cd dist && sha256sum "pogo_${version}_"*.tar.gz > checksums.txt)
fi
