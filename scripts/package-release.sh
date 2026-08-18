#!/bin/sh

set -eu

fail() {
	printf '%s\n' "Cannot package prosecheck: $1" >&2
	exit 1
}

[ "$#" -eq 1 ] || fail "provide one version tag, such as v0.1.0."
tag=$1

printf '%s\n' "$tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$' || fail "the version tag must use Semantic Versioning and start with v."
version=${tag#v}

script_directory=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH='' cd -- "${script_directory}/.." && pwd)
distribution_directory=${PROSECHECK_DIST_DIR:-"${repository_root}/dist"}

if [ -d "$distribution_directory" ] && [ -n "$(ls -A "$distribution_directory")" ]; then
	fail "${distribution_directory} is not empty."
fi

mkdir -p "$distribution_directory"
temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/prosecheck-package.XXXXXX") || fail "a temporary directory could not be created."
trap 'rm -rf "$temporary_directory"' EXIT HUP INT TERM

build_archive() {
	operating_system=$1
	architecture=$2
	executable=prosecheck
	archive="prosecheck_${operating_system}_${architecture}.tar.gz"

	if [ "$operating_system" = windows ]; then
		executable=prosecheck.exe
		archive="prosecheck_${operating_system}_${architecture}.zip"
	fi

	stage_directory="${temporary_directory}/${operating_system}_${architecture}"
	mkdir -p "$stage_directory"

	printf 'Building %s/%s...\n' "$operating_system" "$architecture"
	(
		cd "$repository_root"
		CGO_ENABLED=0 GOOS=$operating_system GOARCH=$architecture go build \
			-trimpath \
			-ldflags "-s -w -X main.version=${version}" \
			-o "${stage_directory}/${executable}" \
			./cmd/prosecheck
	)
	cp "${repository_root}/README.md" "${stage_directory}/README.md"

	if [ "$operating_system" = windows ]; then
		(cd "$stage_directory" && zip -q "${distribution_directory}/${archive}" "$executable" README.md)
	else
		chmod 0755 "${stage_directory}/${executable}"
		tar -C "$stage_directory" -czf "${distribution_directory}/${archive}" "$executable" README.md
	fi
}

build_archive darwin amd64
build_archive darwin arm64
build_archive linux amd64
build_archive linux arm64
build_archive windows amd64
build_archive windows arm64

(
	cd "$distribution_directory"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum prosecheck_*.tar.gz prosecheck_*.zip > checksums.txt
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 prosecheck_*.tar.gz prosecheck_*.zip > checksums.txt
	else
		fail "sha256sum or shasum is required."
	fi
)

printf 'Created release packages for %s in %s.\n' "$tag" "$distribution_directory"
