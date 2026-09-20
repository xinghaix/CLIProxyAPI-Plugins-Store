#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
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

source_file=""
for cand in "$script_dir/bin/${goos}/${goarch}/cursor-oauth.${ext}" \
            "$script_dir/bin/${goos}/${goarch}/cursor.${ext}" \
            "$script_dir/../release/cursor-oauth-v"*"-linux-amd64.${ext}" \
            "$script_dir/cursor-oauth.${ext}"; do
	if [ -f "$cand" ]; then
		source_file="$cand"
		break
	fi
done

[ -n "$source_file" ] && [ -f "$source_file" ] || { echo "plugin binary is unavailable for ${goos}/${goarch}" >&2; exit 1; }

target_dir="$plugins_dir/${goos}/${goarch}"
target_file="$target_dir/cursor-oauth.${ext}"
backup_dir="$plugins_dir/.cursor-oauth-backups"
mkdir -p "$target_dir" "$backup_dir"

if [ -f "$target_file" ]; then
	backup_file="$backup_dir/cursor-oauth-$(date -u +%Y%m%dT%H%M%SZ).${ext}"
	cp -p "$target_file" "$backup_file"
	echo "previous plugin backed up to $backup_file"
fi

temporary_file="$target_file.tmp.$$"
trap 'rm -f "$temporary_file"' EXIT HUP INT TERM
install -m 0755 "$source_file" "$temporary_file"
mv -f "$temporary_file" "$target_file"
trap - EXIT HUP INT TERM

echo "installed cursor-oauth plugin at $target_file"
echo "enable plugins.enabled and plugins.configs.cursor-oauth.enabled, then restart CLIProxyAPI"
