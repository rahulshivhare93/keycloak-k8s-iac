#!/usr/bin/env bash
# Prints the Keycloak admin credentials (read from encrypted Pulumi state).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

pulumi_login
cd "${PULUMI_DIR}"

URL="$(pulumi stack output keycloakURL --stack "${STACK}" 2>/dev/null || true)"
USER="$(pulumi stack output keycloakAdminUser --stack "${STACK}" 2>/dev/null || true)"
PASS="$(pulumi stack output keycloakAdminPassword --show-secrets --stack "${STACK}" 2>/dev/null || true)"

if [[ -z "${PASS}" ]]; then
  err "No credentials found. Has the stack been deployed? Run 'make up'."
  exit 1
fi

cat <<EOF

${BOLD}Keycloak admin credentials${RESET}
  Admin console : ${URL}/admin/
  Username      : ${USER}
  Password      : ${PASS}
EOF
