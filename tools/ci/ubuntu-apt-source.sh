#!/bin/bash
# Select a reachable Ubuntu-signed archive for ephemeral GitHub runners.
set -euo pipefail
check_only=false
case "${1:-}" in
  '') ;;
  --check) check_only=true ;;
  *) echo 'Usage: ubuntu-apt-source.sh [--check]' >&2; exit 2 ;;
esac
if ! $check_only && { test "${GITHUB_ACTIONS:-}" != true || test "${RUNNER_OS:-}" != Linux; }; then
  echo 'APT configuration is limited to ephemeral GitHub Linux runners' >&2
  exit 2
fi
. /etc/os-release
test "$ID" = ubuntu && test "${VERSION_CODENAME:-}" = noble || { echo 'Expected Ubuntu 24.04 runner' >&2; exit 2; }
keyring=/usr/share/keyrings/ubuntu-archive-keyring.gpg
test -f "$keyring"
metadata=$(mktemp)
trap 'rm -f -- "$metadata"' EXIT
selected=''
for origin in https://archive.ubuntu.com/ubuntu https://mirrors.edge.kernel.org/ubuntu; do
  host=${origin#https://}; host=${host%%/*}
  if ! timeout 5s getent ahostsv4 "$host" >/dev/null; then
    echo "Archive DNS unavailable: $host" >&2
    continue
  fi
  verified=true
  for suite in noble noble-security; do
    if ! curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 5 --max-time 20 "$origin/dists/$suite/InRelease" -o "$metadata" ||
       ! gpgv --keyring "$keyring" "$metadata" >/dev/null 2>&1 ||
       ! grep -qx "Suite: $suite" "$metadata" ||
       ! grep -qx 'Codename: noble' "$metadata"; then
      verified=false
      break
    fi
  done
  if $verified; then selected=$origin; break; fi
done
if test -z "$selected"; then
  echo 'No reachable Ubuntu-signed archive; dependency checks cannot run' >&2
  exit 1
fi
printf 'Verified Ubuntu archive: %s\n' "$selected"
if $check_only; then exit 0; fi
test -f /etc/apt/apt-mirrors.txt || { echo 'Expected runner mirror list' >&2; exit 2; }
printf '%s\n' "$selected" | sudo tee /etc/apt/apt-mirrors.txt >/dev/null
if test -f /etc/apt/apt-security-mirrors.txt; then
  printf '%s\n' "$selected" | sudo tee /etc/apt/apt-security-mirrors.txt >/dev/null
fi
# APT retains its installed Ubuntu trust root, package signatures and expiry checks.
