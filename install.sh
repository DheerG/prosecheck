#!/bin/sh

set -eu

repository=${PROSECHECK_REPOSITORY:-DheerG/prosecheck}
install_dir=${PROSECHECK_INSTALL_DIR:-"${HOME}/.local/bin"}
requested_version=${PROSECHECK_VERSION:-}

fail() {
	printf '%s\n' "Cannot install prosecheck: $1" >&2
	exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required."
command -v tar >/dev/null 2>&1 || fail "tar is required."

case $(uname -s) in
	Darwin) operating_system=darwin ;;
	Linux) operating_system=linux ;;
	*) fail "this operating system is not supported." ;;
esac

case $(uname -m) in
	x86_64|amd64) architecture=amd64 ;;
	arm64|aarch64) architecture=arm64 ;;
	*) fail "this processor is not supported." ;;
esac

if [ -n "$requested_version" ]; then
	case $requested_version in
		v*) tag=$requested_version ;;
		*) tag=v$requested_version ;;
	esac
else
	latest_url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${repository}/releases/latest") || fail "the latest release was not found."
	tag=${latest_url##*/}
	[ -n "$tag" ] || fail "the latest release tag was empty."
fi

asset="prosecheck_${operating_system}_${architecture}.tar.gz"
download_root="https://github.com/${repository}/releases/download/${tag}"
temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/prosecheck-install.XXXXXX") || fail "a temporary directory could not be created."
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM

printf 'Downloading prosecheck %s for %s/%s...\n' "$tag" "$operating_system" "$architecture"
curl -fL --progress-bar "${download_root}/${asset}" -o "${temporary_dir}/${asset}" || fail "the release archive could not be downloaded."
curl -fsSL "${download_root}/checksums.txt" -o "${temporary_dir}/checksums.txt" || fail "the checksums could not be downloaded."

expected=$(awk -v asset="$asset" '$2 == asset { print $1 }' "${temporary_dir}/checksums.txt")
[ -n "$expected" ] || fail "the release does not contain a checksum for ${asset}."

if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "${temporary_dir}/${asset}" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "${temporary_dir}/${asset}" | awk '{ print $1 }')
else
	fail "sha256sum or shasum is required."
fi

[ "$actual" = "$expected" ] || fail "the release archive checksum does not match."

mkdir -p "$install_dir" || fail "the install directory could not be created."
tar -xzf "${temporary_dir}/${asset}" -C "$temporary_dir" prosecheck || fail "the release archive could not be opened."
cp "${temporary_dir}/prosecheck" "${install_dir}/prosecheck" || fail "the executable could not be copied."
chmod 0755 "${install_dir}/prosecheck" || fail "the executable permission could not be set."

printf 'Installed prosecheck %s at %s/prosecheck.\n' "$tag" "$install_dir"
case :${PATH:-}: in
	*:"$install_dir":*) ;;
	*) printf 'Add %s to PATH, then run: prosecheck init\n' "$install_dir" ;;
esac
