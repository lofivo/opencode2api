#!/bin/sh
# Build the local opencode2api binary. Run with --help for supported options.
set -eu

root=$(CDPATH= cd "$(dirname "$0")" && pwd)
cd "$root"

usage() {
    cat <<'EOF'
Usage: ./build.sh

Build ./cmd/opencode2api. The output is written atomically.

Environment:
  BINARY        Output path (default: ./opencode2api).
  VERSION       Version embedded by the linker (default: git describe or dev).
  RELEASE       Set to 1 to strip symbols and DWARF data.
  CGO_ENABLED   Defaults to 0 for a self-contained binary.
  GOOS/GOARCH   Optional cross-compilation target.

Examples:
  ./build.sh
  RELEASE=1 VERSION=v1.2.3 ./build.sh
  GOOS=linux GOARCH=arm64 BINARY=dist/opencode2api-linux-arm64 ./build.sh
EOF
}

case ${1:-} in
    -h|--help)
        usage
        exit 0
        ;;
    "")
        ;;
    *)
        printf 'unknown argument: %s\n' "$1" >&2
        usage >&2
        exit 2
        ;;
esac

if ! command -v go >/dev/null 2>&1; then
    printf 'build.sh: go was not found in PATH (Go 1.24 or newer is required)\n' >&2
    exit 1
fi

binary=${BINARY:-$root/opencode2api}
case $binary in
    /*) ;;
    *) binary=$root/$binary ;;
esac

version=${VERSION:-}
if [ -z "$version" ]; then
    version=$(git -C "$root" describe --tags --always --dirty 2>/dev/null || true)
    [ -n "$version" ] || version=dev
fi

ldflags="-X main.version=$version"
if [ "${RELEASE:-0}" = "1" ]; then
    ldflags="-s -w $ldflags"
fi

mkdir -p "$(dirname "$binary")"
tmp="${binary}.tmp.$$"
trap 'rm -f "$tmp"' 0 HUP INT TERM

printf 'building %s (%s)...\n' "$binary" "$version"
CGO_ENABLED=${CGO_ENABLED:-0} go build -trimpath -ldflags "$ldflags" -o "$tmp" ./cmd/opencode2api
mv -f "$tmp" "$binary"
trap - 0 HUP INT TERM

printf 'built %s\n' "$binary"
