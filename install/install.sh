#!/bin/sh
set -eu

repo="${CHAT_TAILS_REPO:-bscott/chat-tails}"
version="${CHAT_TAILS_VERSION:-latest}"
install_dir="${CHAT_TAILS_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
  printf 'chat-tails installer: %s\n' "$*" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

asset="chat-tails-${os}-${arch}"
case "$version" in
  latest)
    release_url="https://github.com/${repo}/releases/latest/download"
    ;;
  v[0-9]*)
    release_url="https://github.com/${repo}/releases/download/${version}"
    ;;
  *)
    fail "CHAT_TAILS_VERSION must be 'latest' or a v-prefixed release tag"
    ;;
esac

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

printf 'Downloading %s from %s...\n' "$asset" "$repo"
curl -fsSL "${release_url}/${asset}" -o "${tmp_dir}/${asset}"
curl -fsSL "${release_url}/SHA256SUMS" -o "${tmp_dir}/SHA256SUMS"

expected=$(awk -v asset="$asset" '$2 == asset { print $1 }' "${tmp_dir}/SHA256SUMS")
[ -n "$expected" ] || fail "${asset} is missing from SHA256SUMS"

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "${tmp_dir}/${asset}" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "${tmp_dir}/${asset}" | awk '{ print $1 }')
else
  fail "sha256sum or shasum is required to verify the download"
fi
[ "$actual" = "$expected" ] || fail "checksum verification failed for ${asset}"

mkdir -p "$install_dir"
chmod 0755 "${tmp_dir}/${asset}"
mv "${tmp_dir}/${asset}" "${install_dir}/chat-tails"

printf 'Installed chat-tails to %s/chat-tails\n' "$install_dir"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) printf 'Add %s to PATH before running chat-tails.\n' "$install_dir" ;;
esac
