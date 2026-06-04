#!/usr/bin/env bash
# End-to-end verification: HTTPS reachability + admin token issuance.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

pulumi_login
cd "${PULUMI_DIR}"

URL="$(pulumi stack output keycloakURL --stack "${STACK}")"
USER="$(pulumi stack output keycloakAdminUser --stack "${STACK}")"
PASS="$(pulumi stack output keycloakAdminPassword --show-secrets --stack "${STACK}")"

# host:port for curl --resolve so we don't depend on /etc/hosts.
HOSTPORT="${URL#https://}"
HOST="${HOSTPORT%%:*}"
PORT="${HOSTPORT##*:}"
RESOLVE="--resolve ${HOST}:${PORT}:127.0.0.1"

log "1/3 HTTPS endpoint reachable (TLS)..."
code="$(curl -sk -o /dev/null -w '%{http_code}' ${RESOLVE} "${URL}/realms/master")"
if [[ "${code}" == "200" ]]; then
  log "    master realm responded 200 over HTTPS ✔"
else
  err "    unexpected HTTP status: ${code}"; exit 1
fi

log "2/3 TLS certificate presented..."
echo | openssl s_client -connect "127.0.0.1:${PORT}" -servername "${HOST}" 2>/dev/null \
  | openssl x509 -noout -subject -issuer 2>/dev/null | sed 's/^/    /' || warn "    could not parse cert"

log "3/3 Admin login (password grant against admin-cli)..."
token="$(curl -sk ${RESOLVE} \
  -d "client_id=admin-cli" \
  -d "username=${USER}" \
  --data-urlencode "password=${PASS}" \
  -d "grant_type=password" \
  "${URL}/realms/master/protocol/openid-connect/token" | grep -o '"access_token"' || true)"

if [[ -n "${token}" ]]; then
  log "    admin authenticated and received an access token ✔"
else
  err "    admin login failed"; exit 1
fi

log "All checks passed. Keycloak is up, HTTPS works, and the admin account is valid."
