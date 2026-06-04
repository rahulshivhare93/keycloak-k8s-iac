#!/usr/bin/env bash
# Best-effort security scanning. Auto-installs scanners via brew/go when missing.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

rc=0

log "Secret scan (gitleaks)..."
if ! have gitleaks; then
  have brew && brew install gitleaks || warn "skipping gitleaks (not installed)"
fi
if have gitleaks; then
  gitleaks detect --source "${ROOT}" --no-banner --redact || { err "gitleaks found potential secrets"; rc=1; }
else
  warn "gitleaks unavailable; skipped"
fi

log "Go vulnerability scan (govulncheck)..."
if ! have govulncheck; then
  go install golang.org/x/vuln/cmd/govulncheck@latest || warn "could not install govulncheck"
  export PATH="$PATH:$(go env GOPATH)/bin"
fi
if have govulncheck || [[ -x "$(go env GOPATH)/bin/govulncheck" ]]; then
  ( cd "${PULUMI_DIR}" && "$(command -v govulncheck || echo "$(go env GOPATH)/bin/govulncheck")" ./... ) \
    || { err "govulncheck reported vulnerabilities"; rc=1; }
else
  warn "govulncheck unavailable; skipped"
fi

if ((rc == 0)); then
  log "Security scans passed."
else
  err "Security scans reported findings (see above)."
fi
exit "${rc}"
