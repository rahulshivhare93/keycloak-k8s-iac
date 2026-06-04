// Command keycloak-k8s-iac is a Pulumi (Go) program that provisions a local
// k3s/Rancher Kubernetes cluster (via k3d) and deploys a hardened, HTTPS-enabled
// Keycloak instance backed by PostgreSQL — fully automated and reproducible.
package main

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	random "github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/rahulshivhare93/keycloak-k8s-iac/internal/cluster"
	"github.com/rahulshivhare93/keycloak-k8s-iac/internal/database"
	"github.com/rahulshivhare93/keycloak-k8s-iac/internal/keycloak"
	"github.com/rahulshivhare93/keycloak-k8s-iac/internal/pki"
	"github.com/rahulshivhare93/keycloak-k8s-iac/internal/security"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "keycloak-k8s-iac")

		clusterName := getOr(cfg, "clusterName", "keycloak-iac")
		namespace := getOr(cfg, "namespace", "keycloak")
		hostname := getOr(cfg, "hostname", "keycloak.localhost")
		adminUser := getOr(cfg, "adminUser", "admin")
		keycloakImage := getOr(cfg, "keycloakImage", "quay.io/keycloak/keycloak:26.0")
		postgresImage := getOr(cfg, "postgresImage", "postgres:16-alpine")
		httpsPort := cfg.GetInt("httpsPort")
		if httpsPort == 0 {
			httpsPort = 8443
		}

		// 1. Provision the local k3d (k3s/Rancher) cluster and obtain its kubeconfig.
		clusterResult, err := cluster.Provision(ctx, clusterName, httpsPort)
		if err != nil {
			return fmt.Errorf("provisioning cluster: %w", err)
		}

		// All in-cluster resources use a provider built from the freshly created
		// cluster's kubeconfig.
		k8s, err := kubernetes.NewProvider(ctx, "k3s", &kubernetes.ProviderArgs{
			Kubeconfig:            clusterResult.Kubeconfig,
			EnableServerSideApply: pulumi.Bool(true),
		}, pulumi.DependsOn([]pulumi.Resource{clusterResult.Resource}))
		if err != nil {
			return fmt.Errorf("creating kubernetes provider: %w", err)
		}
		withProvider := pulumi.Provider(k8s)

		// 2. Namespace that everything lives in.
		ns, err := corev1.NewNamespace(ctx, "keycloak-ns", &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(namespace),
				Labels: pulumi.StringMap{
					"app.kubernetes.io/part-of": pulumi.String("keycloak"),
					// Enables Pod Security Admission "restricted" enforcement.
					"pod-security.kubernetes.io/enforce": pulumi.String("baseline"),
					"pod-security.kubernetes.io/audit":   pulumi.String("restricted"),
				},
			},
		}, withProvider)
		if err != nil {
			return fmt.Errorf("creating namespace: %w", err)
		}
		nsDep := pulumi.DependsOn([]pulumi.Resource{ns})

		// 3. Generate all credentials at deploy time — nothing is stored in the repo.
		dbPassword, err := random.NewRandomPassword(ctx, "postgres-password", &random.RandomPasswordArgs{
			Length:          pulumi.Int(28),
			Special:         pulumi.Bool(true),
			OverrideSpecial: pulumi.String("-_"),
		})
		if err != nil {
			return fmt.Errorf("generating postgres password: %w", err)
		}
		adminPassword, err := random.NewRandomPassword(ctx, "keycloak-admin-password", &random.RandomPasswordArgs{
			Length:          pulumi.Int(24),
			Special:         pulumi.Bool(true),
			OverrideSpecial: pulumi.String("-_.!@"),
		})
		if err != nil {
			return fmt.Errorf("generating admin password: %w", err)
		}

		// 4. Self-signed TLS material for HTTPS access to the Ingress.
		tlsResult, err := pki.NewSelfSignedTLS(ctx, pki.Args{
			Namespace:  namespace,
			Hostname:   hostname,
			SecretName: "keycloak-tls",
		}, withProvider, nsDep)
		if err != nil {
			return fmt.Errorf("creating tls material: %w", err)
		}

		// 5. PostgreSQL backend (internal only, never exposed outside the cluster).
		db, err := database.Deploy(ctx, database.Args{
			Namespace: namespace,
			Image:     postgresImage,
			DBName:    "keycloak",
			Username:  "keycloak",
			Password:  dbPassword.Result,
		}, withProvider, nsDep)
		if err != nil {
			return fmt.Errorf("deploying postgres: %w", err)
		}

		// 6. Keycloak itself + HTTPS Ingress.
		kc, err := keycloak.Deploy(ctx, keycloak.Args{
			Namespace:     namespace,
			Image:         keycloakImage,
			Hostname:      hostname,
			HTTPSPort:     httpsPort,
			AdminUser:     adminUser,
			AdminPassword: adminPassword.Result,
			DBHost:        db.ServiceName,
			DBName:        "keycloak",
			DBUser:        "keycloak",
			DBPassword:    dbPassword.Result,
			TLSSecretName: tlsResult.SecretName,
		}, withProvider, pulumi.DependsOn([]pulumi.Resource{db.Resource, tlsResult.Resource}))
		if err != nil {
			return fmt.Errorf("deploying keycloak: %w", err)
		}

		// 7. Default-deny network hardening for the namespace.
		if err := security.ApplyNetworkPolicies(ctx, security.Args{
			Namespace: namespace,
		}, withProvider, nsDep); err != nil {
			return fmt.Errorf("applying network policies: %w", err)
		}

		// Stack outputs. The admin password is marked secret and is never printed
		// unless explicitly requested with `--show-secrets`.
		ctx.Export("keycloakURL", pulumi.Sprintf("https://%s:%d", hostname, httpsPort))
		ctx.Export("keycloakAdminUser", pulumi.String(adminUser))
		ctx.Export("keycloakAdminPassword", pulumi.ToSecret(adminPassword.Result))
		ctx.Export("namespace", pulumi.String(namespace))
		ctx.Export("clusterName", pulumi.String(clusterName))
		ctx.Export("ingressReady", kc.Ingress.Metadata.Name())

		return nil
	})
}

// getOr returns the configured value for key, or def when unset.
func getOr(cfg *config.Config, key, def string) string {
	if v := cfg.Get(key); v != "" {
		return v
	}
	return def
}
