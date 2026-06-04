#!/usr/bin/env bash
# Provisions the cluster and deploys Keycloak via Pulumi.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

"${ROOT}/scripts/preflight.sh"

log "Resolving Go modules..."
( cd "${PULUMI_DIR}" && go mod download )

log "Configuring Pulumi (local backend, stack: ${STACK})..."
pulumi_login
pulumi_stack

log "Deploying — this provisions a k3d cluster and Keycloak (first run pulls images, ~3-6 min)..."
( cd "${PULUMI_DIR}" && pulumi up --yes --stack "${STACK}" )

log "Deployment complete."
"${ROOT}/scripts/credentials.sh" || true
cat <<EOF

${BOLD}Next steps:${RESET}
  - Add a hosts entry (one-time):   ${BOLD}make hosts${RESET}
  - Open Keycloak:                  $( cd "${PULUMI_DIR}" && pulumi stack output keycloakURL --stack "${STACK}" )
  - Verify end-to-end:              ${BOLD}make verify${RESET}

The TLS certificate is self-signed, so your browser will show a warning — that is expected for a local setup.
EOF
