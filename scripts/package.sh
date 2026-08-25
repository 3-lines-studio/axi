#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
rm -rf dist
mkdir dist
npm --prefix frontend ci
npm --prefix frontend run build

checksum() {
    for file in "$@"; do
        if command -v sha256sum >/dev/null 2>&1; then
            hash=$(sha256sum "$file" | awk '{print $1}')
        elif command -v shasum >/dev/null 2>&1; then
            hash=$(shasum -a 256 "$file" | awk '{print $1}')
        elif command -v openssl >/dev/null 2>&1; then
            hash=$(openssl dgst -sha256 "$file" | awk '{print $NF}')
        else
            echo "package: sha256sum, shasum, or openssl is required" >&2
            return 1
        fi
        printf '%s  %s\n' "$hash" "$file"
    done
}

build() {
    os=$1
    arch=$2
    target=$3
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -tags headless -trimpath -ldflags="-s -w" -o "dist/axi-$target" .
    checksum "dist/axi-$target" > "dist/axi-$target.sha256"
}

build linux amd64 linux-x86_64
build linux arm64 linux-aarch64
build darwin arm64 darwin-aarch64

cd dist
checksum axi-linux-x86_64 axi-linux-aarch64 axi-darwin-aarch64 > SHA256SUMS
