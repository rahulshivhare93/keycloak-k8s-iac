#!/usr/bin/env bash
# Shared helpers and configuration for all scripts.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PULUMI_DIR="${ROOT}/pulumi"
STATE_DIR="${ROOT}/.pulumi-state"
STACK="${PULUMI_STACK:-dev}"

# Self-contained, local Pulumi backend and secrets provider so a reviewer needs
# no Pulumi Cloud account. For production, use a cloud backend + KMS secrets
# provider and a real passphrase.
export PULUMI_CONFIG_PASSPHRASE="${PULUMI_CONFIG_PASSPHRASE:-}"
export PULUMI_SKIP_UPDATE_CHECK=true

# Colors (no-op when not a TTY).
if [[ -t 1 ]]; then
  BOLD="$(printf '\033[1m')"; GREEN="$(printf '\033[32m')"; YELLOW="$(printf '\033[33m')"
  RED="$(printf '\033[31m')"; RESET="$(printf '\033[0m')"
else
  BOLD=""; GREEN=""; YELLOW=""; RED=""; RESET=""
fi

log()  { printf '%s==>%s %s\n' "${BOLD}${GREEN}" "${RESET}" "$*"; }
warn() { printf '%s==>%s %s\n' "${BOLD}${YELLOW}" "${RESET}" "$*"; }
err()  { printf '%s==>%s %s\n' "${BOLD}${RED}" "${RESET}" "$*" >&2; }

have() { command -v "$1" >/dev/null 2>&1; }

pulumi_login() {
  mkdir -p "${STATE_DIR}"
  pulumi login "file://${STATE_DIR}" >/dev/null
}

pulumi_stack() {
  ( cd "${PULUMI_DIR}" && pulumi stack select "${STACK}" 2>/dev/null \
      || pulumi stack init "${STACK}" )
}
