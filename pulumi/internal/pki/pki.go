// Package pki generates a self-signed certificate authority and a server
// certificate for the Keycloak hostname, and stores them in a Kubernetes TLS
// secret consumed by the Ingress. This gives encrypted (HTTPS) access without
// depending on an external CA.
//
// For a production deployment, swap this for cert-manager with an ACME / internal
// CA issuer (see README). The Ingress contract (a kubernetes.io/tls secret) is
// identical, so only this package changes.
package pki

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	tls "github.com/pulumi/pulumi-tls/sdk/v5/go/tls"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Args configures the self-signed TLS material.
type Args struct {
	Namespace  string
	Hostname   string
	SecretName string
}

// Result references the created TLS secret.
type Result struct {
	SecretName string
	Resource   pulumi.Resource
}

// NewSelfSignedTLS creates a CA, a hostname-scoped server certificate signed by
// that CA, and a kubernetes.io/tls secret containing the server cert chain.
func NewSelfSignedTLS(ctx *pulumi.Context, args Args, opts ...pulumi.ResourceOption) (*Result, error) {
	caKey, err := tls.NewPrivateKey(ctx, "ca-key", &tls.PrivateKeyArgs{
		Algorithm: pulumi.String("RSA"),
		RsaBits:   pulumi.Int(4096),
	})
	if err != nil {
		return nil, err
	}

	caCert, err := tls.NewSelfSignedCert(ctx, "ca-cert", &tls.SelfSignedCertArgs{
		PrivateKeyPem:       caKey.PrivateKeyPem,
		IsCaCertificate:     pulumi.Bool(true),
		ValidityPeriodHours: pulumi.Int(8760), // 1 year
		Subject: &tls.SelfSignedCertSubjectArgs{
			CommonName:   pulumi.String("keycloak-local-ca"),
			Organization: pulumi.String("keycloak-k8s-iac"),
		},
		AllowedUses: pulumi.StringArray{
			pulumi.String("cert_signing"),
			pulumi.String("crl_signing"),
		},
	})
	if err != nil {
		return nil, err
	}

	serverKey, err := tls.NewPrivateKey(ctx, "server-key", &tls.PrivateKeyArgs{
		Algorithm: pulumi.String("RSA"),
		RsaBits:   pulumi.Int(2048),
	})
	if err != nil {
		return nil, err
	}

	csr, err := tls.NewCertRequest(ctx, "server-csr", &tls.CertRequestArgs{
		PrivateKeyPem: serverKey.PrivateKeyPem,
		Subject: &tls.CertRequestSubjectArgs{
			CommonName:   pulumi.String(args.Hostname),
			Organization: pulumi.String("keycloak-k8s-iac"),
		},
		DnsNames: pulumi.StringArray{
			pulumi.String(args.Hostname),
		},
	})
	if err != nil {
		return nil, err
	}

	serverCert, err := tls.NewLocallySignedCert(ctx, "server-cert", &tls.LocallySignedCertArgs{
		CertRequestPem:      csr.CertRequestPem,
		CaPrivateKeyPem:     caKey.PrivateKeyPem,
		CaCertPem:           caCert.CertPem,
		ValidityPeriodHours: pulumi.Int(8760),
		AllowedUses: pulumi.StringArray{
			pulumi.String("server_auth"),
			pulumi.String("key_encipherment"),
			pulumi.String("digital_signature"),
		},
	})
	if err != nil {
		return nil, err
	}

	// Full chain (server + CA) so clients that trust the CA validate cleanly.
	fullChain := pulumi.All(serverCert.CertPem, caCert.CertPem).ApplyT(
		func(parts []interface{}) string {
			return parts[0].(string) + parts[1].(string)
		}).(pulumi.StringOutput)

	secret, err := corev1.NewSecret(ctx, "keycloak-tls", &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(args.SecretName),
			Namespace: pulumi.String(args.Namespace),
		},
		Type: pulumi.String("kubernetes.io/tls"),
		StringData: pulumi.StringMap{
			"tls.crt": fullChain,
			"tls.key": serverKey.PrivateKeyPem,
			"ca.crt":  caCert.CertPem,
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	return &Result{
		SecretName: args.SecretName,
		Resource:   secret,
	}, nil
}
