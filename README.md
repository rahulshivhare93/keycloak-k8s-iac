# Keycloak on Local Kubernetes — Pulumi + Go IaC

A fully automated, reproducible deployment of [Keycloak](https://www.keycloak.org/)
on a local Kubernetes cluster, defined entirely as **Infrastructure as Code with
Pulumi and Go**.

A single command provisions a [k3d](https://k3d.io/) cluster (k3d runs **k3s** —
the lightweight Kubernetes distribution that powers **Rancher**), then deploys a
hardened Keycloak instance backed by PostgreSQL, reachable over **HTTPS** with a
pre-configured **admin** account.

---

## Table of contents

- [Architecture](#architecture)
- [Prerequisites & assumptions](#prerequisites--assumptions)
- [Quick start](#quick-start)
- [Make targets](#make-targets)
- [Accessing Keycloak](#accessing-keycloak)
- [Verifying the deployment](#verifying-the-deployment)
- [Security & hardening](#security--hardening)
- [How credentials are handled](#how-credentials-are-handled)
- [Project layout](#project-layout)
- [Tear down](#tear-down)
- [Time spent](#time-spent)

---

## Architecture

```
                         host: https://keycloak.localhost:8443
                                        │
                                        ▼  (host port 8443 → cluster 443)
┌──────────────────────────── k3d / k3s cluster ───────────────────────────┐
│                                                                           │
│   kube-system: Traefik Ingress  ──TLS termination (self-signed cert)──┐   │
│                                                                       │   │
│   namespace: keycloak                                                 ▼   │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │  Ingress (TLS) ──▶ Service:8080 ──▶ Keycloak Deployment          │   │
│   │                                         │ (non-root, caps dropped)│   │
│   │                                         ▼ 5432                     │   │
│   │                              Service ──▶ PostgreSQL Deployment     │   │
│   │                                            + PersistentVolume      │   │
│   │                                                                    │   │
│   │  NetworkPolicies: default-deny + minimal allow (DNS, ingress, db)  │   │
│   └─────────────────────────────────────────────────────────────────┘   │
└───────────────────────────────────────────────────────────────────────────┘
```

Everything in the diagram — including the cluster itself — is created by
`pulumi up`. The Pulumi program (Go) is the single source of truth.

| Component        | Choice                                   | Why |
|------------------|------------------------------------------|-----|
| IaC tool         | **Pulumi + Go**                          | Requested; real language, testable, typed |
| Cluster          | **k3d** (k3s = Rancher's distro)         | "Rancher preferred"; fast, lightweight, local |
| Container runtime| **colima** (auto-installed)              | Docker daemon on macOS without Docker Desktop licensing |
| Identity server  | **Keycloak 26**                          | The assignment target |
| Database         | **PostgreSQL 16**                        | Production-style persistence (not ephemeral H2) |
| TLS              | Self-signed cert via Pulumi `tls`        | HTTPS with zero external dependencies |
| Ingress          | **Traefik** (built into k3s)             | TLS termination, no extra install |
| Secrets          | Pulumi `random` + encrypted state        | No plaintext credentials in the repo |

---

## Prerequisites & assumptions

- **macOS** (tested on Apple Silicon, macOS 14+). The automation targets macOS.
- **[Homebrew](https://brew.sh)** installed — used to auto-install any missing tools.
- Internet access on first run (to pull container images and Go modules).
- Ports **8443** (HTTPS) free on the host.
- Everything else (`pulumi`, `k3d`, `kubectl`, `docker`/`colima`, `go`) is
  **installed automatically** by `make preflight` if missing.

No Pulumi Cloud account is required — the stack uses a **local file backend**
stored under `.pulumi-state/` (git-ignored).

---

## Quick start

```bash
git clone <this-repo-url>
cd keycloak-k8s-iac

# One command: installs tooling, starts Docker, provisions the cluster,
# deploys Keycloak + PostgreSQL, and prints the admin credentials.
make up

# (one-time) make the hostname resolvable in your browser
make hosts
```

First run takes ~3–6 minutes (image pulls + Keycloak build). When it finishes it
prints the URL and admin credentials. Open the **Admin console** link, accept the
self-signed certificate warning, and log in.

> Prefer to do it yourself step by step? Run `make preflight`, then
> `cd pulumi && pulumi login file://$(pwd)/../.pulumi-state && pulumi stack init dev && pulumi up`.

---

## Make targets

Everything is driven through the `Makefile`. Run `make` (or `make help`) to list
every target. All commands are run from the repository root.

| Command          | What it does |
|------------------|--------------|
| `make help`      | List all targets (this is the default when you run `make`). |
| `make preflight` | Install required tooling via Homebrew and start the colima Docker runtime. Idempotent — safe to re-run. |
| `make up`        | **One-shot setup:** preflight → provision the k3d cluster → deploy TLS, PostgreSQL and Keycloak → print credentials. |
| `make creds`     | Print the Keycloak admin username and the (decrypted) admin password. |
| `make url`       | Print the Keycloak URL. |
| `make verify`    | End-to-end check: HTTPS 200 on the master realm, the TLS certificate, and a real admin login (OIDC password grant). |
| `make status`    | Show pods, services, ingress and network policies in the `keycloak` namespace. |
| `make hosts`     | Add `127.0.0.1 keycloak.localhost` to `/etc/hosts` so the hostname resolves in a browser (uses `sudo`). |
| `make destroy`   | Tear down Keycloak and delete the k3d cluster. |
| `make fmt`       | Format the Go code (`gofmt`) and tidy modules. |
| `make vet`       | Run `go vet`. |
| `make lint`      | Run `golangci-lint` (auto-installs it via Homebrew if missing). |
| `make build`     | Compile the Pulumi program. |
| `make scan`      | Security scans: `gitleaks` (secret detection) + `govulncheck` (Go CVEs). |
| `make check`     | `fmt` + `vet` + `build` — the pre-commit gate. |

**Overridable variables**

- `HOSTNAME` — the Keycloak hostname used by `make hosts` (default `keycloak.localhost`).
  Example: `make hosts HOSTNAME=id.localhost`.

**Common workflows**

```bash
make up && make hosts && make verify   # deploy, enable browser DNS, prove it works
make creds                             # grab the admin login
make check                             # before committing changes
make destroy                           # clean everything up
```

---

## Accessing Keycloak

| What            | Where |
|-----------------|-------|
| Account console | `https://keycloak.localhost:8443/` |
| Admin console   | `https://keycloak.localhost:8443/admin/` |

Print the credentials at any time:

```bash
make creds
```

The admin **username** is `admin` (configurable). The **password** is randomly
generated at deploy time and stored encrypted in Pulumi state — retrieve it with
`make creds` (which runs `pulumi stack output keycloakAdminPassword --show-secrets`).

> The certificate is self-signed, so browsers show a warning on first visit. That
> is expected for a local environment — proceed past it.

---

## Verifying the deployment

```bash
make verify
```

This automated check:
1. Confirms the master realm responds **200 over HTTPS**.
2. Prints the presented **TLS certificate** subject/issuer.
3. Performs a real **admin login** (OIDC password grant) and confirms a token is issued.

You can also inspect the workloads:

```bash
make status   # pods, services, ingress, network policies in the keycloak namespace
```

---

## Security & hardening

This deployment applies "basic hardening" and then some:

**Encrypted access**
- HTTPS only at the edge (Traefik `websecure` entrypoint) with a self-signed cert.
- Keycloak is told its public URL is `https://...` and trusts `X-Forwarded-*`
  headers (`KC_PROXY_HEADERS=xforwarded`) — the standard edge-TLS pattern.

**Minimal network exposure**
- Only the Keycloak HTTP port is reachable, and only **through the Ingress**.
- **Default-deny** `NetworkPolicy` for all ingress/egress in the namespace, then
  minimal allow rules:
  - all pods → kube-dns (port 53) only
  - Keycloak ← ingress controller only (ports 8080/9000)
  - Keycloak → PostgreSQL only (port 5432)
  - PostgreSQL ← Keycloak only
- PostgreSQL is a `ClusterIP` service — **never** exposed outside the cluster.

**Workload hardening (pod/container)**
- `runAsNonRoot`, explicit non-root UID/GID, `fsGroup`.
- `allowPrivilegeEscalation: false`, **all Linux capabilities dropped**.
- `seccompProfile: RuntimeDefault`.
- CPU/memory **requests and limits** on every container.
- `automountServiceAccountToken: false` (workloads don't talk to the API server).
- Liveness/readiness/startup **health probes** on Keycloak's management port.
- Namespace labelled for **Pod Security Admission**.

**Credential hygiene**
- All secrets are generated at deploy time by Pulumi's `random` provider.
- Stored in Kubernetes `Secret`s and in **encrypted** Pulumi state.
- **No plaintext credentials anywhere in the repository.**

**Supply chain / code quality**
- Pinned image tags and pinned Go module versions (`go.sum`).
- CI runs `gofmt`, `go vet`, `golangci-lint`, **govulncheck**, and **gitleaks**
  (see `.github/workflows/ci.yml`). Run locally with `make scan`.

---

## How credentials are handled

There are **no secrets committed to this repo**. The flow is:

1. `pulumi up` calls the `random` provider to generate the PostgreSQL password and
   the Keycloak admin password.
2. These are written into Kubernetes `Secret`s (consumed via `envFrom`/`secretKeyRef`)
   and recorded in Pulumi state, **encrypted** by the stack's secrets provider.
3. The admin password is exported as a **secret** stack output — it is never shown
   in logs unless you explicitly run `make creds` / `--show-secrets`.

For this local setup the secrets provider uses a passphrase (empty by default, via
`PULUMI_CONFIG_PASSPHRASE`). For production, switch to a cloud KMS secrets provider
(see [Production notes](#production-notes)).

---

## Project layout

```
keycloak-k8s-iac/
├── Makefile                  # task runner (make help)
├── README.md
├── scripts/                  # bash automation used by the Makefile
│   ├── lib.sh                # shared config/helpers
│   ├── preflight.sh          # install tools + start Docker runtime
│   ├── deploy.sh             # provision + deploy (make up)
│   ├── credentials.sh        # print admin creds
│   ├── verify.sh             # end-to-end checks
│   ├── destroy.sh            # tear everything down
│   └── scan.sh               # security scans
├── pulumi/                   # the IaC program (Pulumi + Go)
│   ├── Pulumi.yaml           # project + config schema
│   ├── go.mod / go.sum
│   ├── main.go               # orchestration
│   ├── .golangci.yml
│   └── internal/
│       ├── cluster/          # k3d cluster provisioning
│       ├── pki/              # self-signed TLS material
│       ├── database/         # PostgreSQL
│       ├── keycloak/         # Keycloak Deployment + Service + Ingress
│       └── security/         # NetworkPolicies
└── .github/workflows/ci.yml  # build, vet, lint, vuln & secret scans
```

Run `make help` to see all available commands.

---

## Tear down

```bash
make destroy   # removes Keycloak, PostgreSQL, and the k3d cluster
```

To also stop the Docker runtime: `colima stop`.


---

## Time spent

**Approximately 6 hours** — design, Pulumi/Go implementation, hardening, the
automation/scripts, end-to-end verification, and documentation.

<!-- Adjust the figure above to reflect your own review/testing time before submitting. -->
