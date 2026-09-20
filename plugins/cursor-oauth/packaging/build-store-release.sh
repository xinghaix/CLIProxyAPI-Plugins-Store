#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd)
version=${1:-0.6.2}
output_dir=${2:-"$project_dir/release"}
plugin_id="cursor-oauth"

mkdir -p "$output_dir"
rm -f "$output_dir/checksums.txt"

package_binary() {
    bin_path="$1"
    goos="$2"
    goarch="$3"
    ext="$4"
    
    lib_name="${plugin_id}-v${version}.${ext}"
    zip_name="${plugin_id}_${version}_${goos}_${goarch}.zip"
    staging=$(mktemp -d)
    
    install -m 0755 "$bin_path" "$staging/$lib_name"
    (cd "$staging" && zip -q -j "$output_dir/$zip_name" "$lib_name")
    rm -rf "$staging"
    
    if command -v shasum >/dev/null 2>&1; then
        (cd "$output_dir" && shasum -a 256 "$zip_name" >> checksums.txt)
    else
        (cd "$output_dir" && sha256sum "$zip_name" >> checksums.txt)
    fi
    echo "Packaged $output_dir/$zip_name"
}

# Check if prebuilt binaries are in a staging directory or build them locally
if [ -n "${CURSOR_PLUGIN_BINARY:-}" ]; then
    goos="${CURSOR_PLUGIN_GOOS:-linux}"
    goarch="${CURSOR_PLUGIN_GOARCH:-amd64}"
    case "$goos" in
        darwin) ext="dylib" ;;
        windows) ext="dll" ;;
        *) ext="so" ;;
    esac
    package_binary "$CURSOR_PLUGIN_BINARY" "$goos" "$goarch" "$ext"
else
    # Build what can be built on current host
    host_os=$(uname -s | tr '[:upper:]' '[:lower:]')
    if [ "$host_os" = "darwin" ]; then
        echo "Building darwin/arm64..."
        (cd "$project_dir/go" && CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -buildmode=c-shared -ldflags="-X main.pluginVersion=${version}" -o "$output_dir/${plugin_id}-v${version}-darwin-arm64.dylib" .)
        package_binary "$output_dir/${plugin_id}-v${version}-darwin-arm64.dylib" "darwin" "arm64" "dylib"
        rm -f "$output_dir/${plugin_id}-v${version}-darwin-arm64.dylib" "$output_dir/${plugin_id}-v${version}-darwin-arm64.h"

        echo "Building darwin/amd64..."
        (cd "$project_dir/go" && CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -buildmode=c-shared -ldflags="-X main.pluginVersion=${version}" -o "$output_dir/${plugin_id}-v${version}-darwin-amd64.dylib" .)
        package_binary "$output_dir/${plugin_id}-v${version}-darwin-amd64.dylib" "darwin" "amd64" "dylib"
        rm -f "$output_dir/${plugin_id}-v${version}-darwin-amd64.dylib" "$output_dir/${plugin_id}-v${version}-darwin-amd64.h"
    elif [ "$host_os" = "linux" ]; then
        echo "Building linux/amd64..."
        (cd "$project_dir/go" && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -buildmode=c-shared -ldflags="-X main.pluginVersion=${version}" -o "$output_dir/${plugin_id}-v${version}-linux-amd64.so" .)
        package_binary "$output_dir/${plugin_id}-v${version}-linux-amd64.so" "linux" "amd64" "so"
        rm -f "$output_dir/${plugin_id}-v${version}-linux-amd64.so" "$output_dir/${plugin_id}-v${version}-linux-amd64.h"
    fi
fi

if [ -f "$output_dir/checksums.txt" ]; then
    echo "Checksums written to $output_dir/checksums.txt"
fi
