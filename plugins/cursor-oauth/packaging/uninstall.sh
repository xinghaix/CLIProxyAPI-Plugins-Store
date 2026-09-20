#!/bin/sh
set -eu

plugins_dir=${CLIPROXY_PLUGINS_DIR:-plugins}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--plugins-dir)
			[ "$#" -ge 2 ] || { echo "--plugins-dir requires a path" >&2; exit 2; }
			plugins_dir=$2
			shift 2
			;;
		--help|-h)
			echo "usage: $0 [--plugins-dir PATH]"
			exit 0
			;;
		*)
			echo "unknown argument: $1" >&2
			exit 2
			;;
	esac
done

case "$(uname -s)" in
	Linux) goos=linux; ext=so ;;
	Darwin) goos=darwin; ext=dylib ;;
	*) echo "unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
	x86_64|amd64) goarch=amd64 ;;
	aarch64|arm64) goarch=arm64 ;;
	*) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

target_file="$plugins_dir/${goos}/${goarch}/cursor-oauth.${ext}"
if [ ! -f "$target_file" ]; then
	target_file="$plugins_dir/${goos}/${goarch}/cursor.${ext}"
fi

[ -f "$target_file" ] || { echo "cursor-oauth plugin is not installed at $target_file"; exit 0; }

removed_dir="$plugins_dir/.cursor-oauth-uninstalled"
mkdir -p "$removed_dir"
removed_file="$removed_dir/cursor-oauth-$(date -u +%Y%m%dT%H%M%SZ).${ext}"
mv "$target_file" "$removed_file"

echo "uninstalled cursor-oauth plugin; recoverable binary moved to $removed_file"
echo "disable plugins.configs.cursor-oauth.enabled, then restart CLIProxyAPI"
