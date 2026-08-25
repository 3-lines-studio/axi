#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
rm -rf dist
mkdir dist
npm --prefix frontend ci
npm --prefix frontend run build
go test -tags headless .

axi_version=${AXI_VERSION:-dev}
ax_version=${AX_VERSION:-v0.4.4}
axis_version=${AXIS_VERSION:-v0.2.0}
fsx_version=${FSX_VERSION:-v0.1.0}
bashx_version=${BASHX_VERSION:-v0.2.1}
skillx_version=${SKILLX_VERSION:-v0.1.0}
attachx_version=${ATTACHX_VERSION:-v0.1.0}

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

download() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$1" -o "$2"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "$1" -O "$2"
    else
        echo "package: curl or wget is required" >&2
        return 1
    fi
}

add_binary() {
    directory=$1
    package=$2
    version=$3
    target=$4
    artifact="$package-$target"
    base="https://github.com/3-lines-studio/$package/releases/download/$version"
    download "$base/$artifact" "$directory/$package"
    download "$base/$artifact.sha256" "$directory/$package.sha256"
    want=$(awk '{print $1}' "$directory/$package.sha256")
    got=$(checksum "$directory/$package" | awk '{print $1}')
    test "$want" = "$got"
    chmod 0755 "$directory/$package"
    rm "$directory/$package.sha256"
}

build() {
    os=$1
    arch=$2
    target=$3
    binary="dist/axi-$target"
    bundle="dist/axi-bundle-$target.tar.gz"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -tags headless -trimpath -ldflags="-s -w -X main.version=$axi_version" -o "$binary" .
    checksum "$binary" > "$binary.sha256"
    directory=$(mktemp -d)
    cp "$binary" "$directory/axi"
    add_binary "$directory" ax "$ax_version" "$target"
    add_binary "$directory" axis "$axis_version" "$target"
    add_binary "$directory" fsx "$fsx_version" "$target"
    add_binary "$directory" bashx "$bashx_version" "$target"
    add_binary "$directory" skillx "$skillx_version" "$target"
    add_binary "$directory" attachx "$attachx_version" "$target"
    tar -C "$directory" -czf "$bundle" axi axis ax fsx bashx skillx attachx
    checksum "$bundle" > "$bundle.sha256"
    rm -rf "$directory"
}

build linux amd64 linux-x86_64
build linux arm64 linux-aarch64
build darwin arm64 darwin-aarch64

cd dist
checksum axi-linux-x86_64 axi-linux-aarch64 axi-darwin-aarch64 \
    axi-bundle-linux-x86_64.tar.gz axi-bundle-linux-aarch64.tar.gz axi-bundle-darwin-aarch64.tar.gz > SHA256SUMS
