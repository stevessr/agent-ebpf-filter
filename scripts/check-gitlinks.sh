#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

tracked=$(mktemp)
mapped=$(mktemp)
path_sections=$(mktemp)
url_sections=$(mktemp)
trap 'rm -f "$tracked" "$mapped" "$path_sections" "$url_sections"' EXIT HUP INT TERM

git ls-files --stage | awk '$1 == "160000" { print $4 }' | LC_ALL=C sort -u >"$tracked"

if [ -s "$tracked" ] && [ ! -f .gitmodules ]; then
	echo "tracked gitlinks require a .gitmodules file" >&2
	exit 1
fi

if [ -f .gitmodules ]; then
	git config -f .gitmodules --get-regexp '^submodule\..*\.path$' |
		awk '{ print $2 }' | LC_ALL=C sort -u >"$mapped"
	git config -f .gitmodules --name-only --get-regexp '^submodule\..*\.path$' |
		sed 's/\.path$//' | LC_ALL=C sort -u >"$path_sections"
	git config -f .gitmodules --name-only --get-regexp '^submodule\..*\.url$' |
		sed 's/\.url$//' | LC_ALL=C sort -u >"$url_sections"
else
	: >"$mapped"
	: >"$path_sections"
	: >"$url_sections"
fi

if ! diff -u "$tracked" "$mapped"; then
	echo "tracked gitlinks and .gitmodules paths differ" >&2
	exit 1
fi

if ! diff -u "$path_sections" "$url_sections"; then
	echo "every submodule path must have a matching URL" >&2
	exit 1
fi

echo "gitlink metadata is complete ($(wc -l <"$tracked" | tr -d ' ') entries)"
