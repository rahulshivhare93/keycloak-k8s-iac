#!/usr/bin/env bash
# Tears down all resources, including the k3d cluster.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

pulumi_login
cd "${PULUMI_DIR}"

log "Destroying stack '${STACK}' (removes Keycloak, PostgreSQL and the k3d cluster)..."
pulumi destroy --yes --stack "${STACK}" || warn "pulumi destroy reported errors; attempting cluster cleanup anyway"

# Belt-and-suspenders: ensure the cluster is gone even if state drifted.
CLUSTER="$(pulumi config get keycloak-k8s-iac:clusterName --stack "${STACK}" 2>/dev/null || echo keycloak-iac)"
if have k3d && k3d cluster list "${CLUSTER}" >/dev/null 2>&1; then
  warn "Removing leftover k3d cluster '${CLUSTER}'..."
  k3d cluster delete "${CLUSTER}" || true
fi

log "Teardown complete."
