#!/usr/bin/env bash
# Ensures all required tooling is installed and a container runtime is running.
# Safe to run repeatedly (idempotent). Targets macOS (Homebrew + colima).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

REQUIRED=(pulumi k3d kubectl docker go)

install_with_brew() {
  if ! have brew; then
    err "Homebrew is required to auto-install dependencies. Install it from https://brew.sh and re-run."
    exit 1
  fi
  log "Installing missing tools with Homebrew: $*"
  brew install "$@"
}

main() {
  log "Checking required tooling..."
  local missing=()
  for t in "${REQUIRED[@]}"; do
    if have "$t"; then
      printf '    %-8s ok\n' "$t"
    else
      printf '    %-8s MISSING\n' "$t"
      missing+=("$t")
    fi
  done

  if ((${#missing[@]} > 0)); then
    # Map tool name -> brew formula.
    local formulae=()
    for t in "${missing[@]}"; do
      case "$t" in
        pulumi) formulae+=("pulumi/tap/pulumi") ;;
        docker) formulae+=("docker" "colima") ;;
        *) formulae+=("$t") ;;
      esac
    done
    install_with_brew "${formulae[@]}"
  fi

  # Ensure a Docker daemon is available; k3d needs one. On macOS we use colima.
  if ! docker info >/dev/null 2>&1; then
    if have colima; then
      log "Starting colima (Docker runtime)..."
      colima start --cpu 4 --memory 6 --disk 40
    else
      err "No running Docker daemon and colima not found. Start Docker Desktop / Rancher Desktop and re-run."
      exit 1
    fi
  fi
  log "Docker runtime is ready."
  log "Preflight complete."
}

main "$@"
