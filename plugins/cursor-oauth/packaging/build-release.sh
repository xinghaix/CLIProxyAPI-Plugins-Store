#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd)
version=${1:-0.6.2}
output_dir=${2:-"$project_dir/release"}
binary=${CURSOR_PLUGIN_BINARY:-"$project_dir/cursor.so"}

[ -f "$binary" ] || { echo "set CURSOR_PLUGIN_BINARY to a Linux amd64 cursor.so" >&2; exit 1; }

package_name="cursor-plugin-$version-linux-amd64"
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT HUP INT TERM
package_dir="$staging/$package_name"
mkdir -p "$package_dir/bin/linux/amd64" "$output_dir"

install -m 0755 "$binary" "$package_dir/bin/linux/amd64/cursor.so"
install -m 0755 "$script_dir/install.sh" "$package_dir/install.sh"
install -m 0755 "$script_dir/uninstall.sh" "$package_dir/uninstall.sh"
cp "$project_dir/README.md" "$package_dir/README.md"
cp "$project_dir/DESIGN.md" "$package_dir/DESIGN.md"
cp "$project_dir/DISCLAIMER.md" "$package_dir/DISCLAIMER.md"
cp "$project_dir/LICENSE" "$package_dir/LICENSE"
cp "$project_dir/THIRD_PARTY_NOTICES.md" "$package_dir/THIRD_PARTY_NOTICES.md"
cp -R "$project_dir/docs" "$package_dir/docs"

if command -v sha256sum >/dev/null 2>&1; then
	(cd "$package_dir" && sha256sum bin/linux/amd64/cursor.so > SHA256SUMS)
else
	(cd "$package_dir" && shasum -a 256 bin/linux/amd64/cursor.so > SHA256SUMS)
fi

COPYFILE_DISABLE=1 tar --no-xattrs -czf "$output_dir/$package_name.tar.gz" -C "$staging" "$package_name"
echo "$output_dir/$package_name.tar.gz"
