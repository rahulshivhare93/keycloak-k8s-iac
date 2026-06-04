// Package keycloak deploys Keycloak (backed by PostgreSQL) together with an
// HTTPS Ingress. TLS is terminated at the edge (Ingress) and Keycloak is told to
// trust forwarded proxy headers, which is the standard, secure pattern for an
// in-cluster identity provider.
package keycloak

import (
	"fmt"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Args configures the Keycloak deployment.
type Args struct {
	Namespace     string
	Image         string
	Hostname      string
	HTTPSPort     int
	AdminUser     string
	AdminPassword pulumi.StringInput
	DBHost        string
	DBName        string
	DBUser        string
	DBPassword    pulumi.StringInput
	TLSSecretName string
}

// Result exposes the created Ingress.
type Result struct {
	Ingress *netv1.Ingress
}

const appName = "keycloak"

// Deploy creates the Keycloak secret, Deployment, Service and HTTPS Ingress.
func Deploy(ctx *pulumi.Context, args Args, opts ...pulumi.ResourceOption) (*Result, error) {
	labels := pulumi.StringMap{
		"app":                       pulumi.String(appName),
		"app.kubernetes.io/name":    pulumi.String(appName),
		"app.kubernetes.io/part-of": pulumi.String("keycloak"),
	}

	// Public URL Keycloak advertises (issuer, redirects, etc.).
	publicURL := fmt.Sprintf("https://%s:%d", args.Hostname, args.HTTPSPort)
	jdbcURL := fmt.Sprintf("jdbc:postgresql://%s:5432/%s", args.DBHost, args.DBName)

	secret, err := corev1.NewSecret(ctx, "keycloak-secrets", &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("keycloak-secrets"),
			Namespace: pulumi.String(args.Namespace),
			Labels:    labels,
		},
		Type: pulumi.String("Opaque"),
		StringData: pulumi.StringMap{
			"KC_BOOTSTRAP_ADMIN_USERNAME": pulumi.String(args.AdminUser),
			"KC_BOOTSTRAP_ADMIN_PASSWORD": args.AdminPassword,
			"KC_DB_PASSWORD":              args.DBPassword,
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	depOpts := append([]pulumi.ResourceOption{
		pulumi.DependsOn([]pulumi.Resource{secret}),
	}, opts...)

	_, err = appsv1.NewDeployment(ctx, "keycloak", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(appName),
			Namespace: pulumi.String(args.Namespace),
			Labels:    labels,
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec: &corev1.PodSpecArgs{
					AutomountServiceAccountToken: pulumi.Bool(false),
					SecurityContext: &corev1.PodSecurityContextArgs{
						RunAsNonRoot: pulumi.Bool(true),
						RunAsUser:    pulumi.Int(1000),
						FsGroup:      pulumi.Int(1000),
						SeccompProfile: &corev1.SeccompProfileArgs{
							Type: pulumi.String("RuntimeDefault"),
						},
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String(appName),
							Image: pulumi.String(args.Image),
							// `start` (non-optimized) auto-builds for the configured
							// DB vendor at boot, keeping the stock image reproducible.
							Args: pulumi.StringArray{
								pulumi.String("start"),
								pulumi.String("--optimized=false"),
							},
							EnvFrom: corev1.EnvFromSourceArray{
								&corev1.EnvFromSourceArgs{
									SecretRef: &corev1.SecretEnvSourceArgs{
										Name: secret.Metadata.Name().Elem(),
									},
								},
							},
							Env: corev1.EnvVarArray{
								env("KC_DB", "postgres"),
								env("KC_DB_URL", jdbcURL),
								env("KC_DB_USERNAME", args.DBUser),
								env("KC_HOSTNAME", publicURL),
								env("KC_HOSTNAME_STRICT", "true"),
								// TLS terminates at the Ingress; trust its headers.
								env("KC_HTTP_ENABLED", "true"),
								env("KC_PROXY_HEADERS", "xforwarded"),
								env("KC_HEALTH_ENABLED", "true"),
								env("KC_METRICS_ENABLED", "true"),
								env("KC_CACHE", "local"),
							},
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									Name:          pulumi.String("http"),
									ContainerPort: pulumi.Int(8080),
								},
								&corev1.ContainerPortArgs{
									Name:          pulumi.String("management"),
									ContainerPort: pulumi.Int(9000),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("250m"),
									"memory": pulumi.String("512Mi"),
								},
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("1000m"),
									"memory": pulumi.String("1Gi"),
								},
							},
							SecurityContext: &corev1.SecurityContextArgs{
								AllowPrivilegeEscalation: pulumi.Bool(false),
								RunAsNonRoot:             pulumi.Bool(true),
								Capabilities: &corev1.CapabilitiesArgs{
									Drop: pulumi.StringArray{pulumi.String("ALL")},
								},
							},
							// Health endpoints are served on the management port (9000).
							StartupProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path: pulumi.String("/health/started"),
									Port: pulumi.Int(9000),
								},
								InitialDelaySeconds: pulumi.Int(15),
								PeriodSeconds:       pulumi.Int(10),
								FailureThreshold:    pulumi.Int(30), // up to ~5 min to boot+build
							},
							ReadinessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path: pulumi.String("/health/ready"),
									Port: pulumi.Int(9000),
								},
								PeriodSeconds:    pulumi.Int(10),
								FailureThreshold: pulumi.Int(3),
							},
							LivenessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path: pulumi.String("/health/live"),
									Port: pulumi.Int(9000),
								},
								PeriodSeconds:    pulumi.Int(15),
								FailureThreshold: pulumi.Int(5),
							},
						},
					},
				},
			},
		},
	}, depOpts...)
	if err != nil {
		return nil, err
	}

	svc, err := corev1.NewService(ctx, "keycloak", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(appName),
			Namespace: pulumi.String(args.Namespace),
			Labels:    labels,
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("ClusterIP"),
			Selector: labels,
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(8080),
					TargetPort: pulumi.Int(8080),
				},
			},
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	ingOpts := append([]pulumi.ResourceOption{
		pulumi.DependsOn([]pulumi.Resource{svc}),
	}, opts...)

	ingress, err := netv1.NewIngress(ctx, "keycloak", &netv1.IngressArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(appName),
			Namespace: pulumi.String(args.Namespace),
			Labels:    labels,
			Annotations: pulumi.StringMap{
				"traefik.ingress.kubernetes.io/router.entrypoints": pulumi.String("websecure"),
				"traefik.ingress.kubernetes.io/router.tls":         pulumi.String("true"),
			},
		},
		Spec: &netv1.IngressSpecArgs{
			IngressClassName: pulumi.String("traefik"),
			Tls: netv1.IngressTLSArray{
				&netv1.IngressTLSArgs{
					Hosts:      pulumi.StringArray{pulumi.String(args.Hostname)},
					SecretName: pulumi.String(args.TLSSecretName),
				},
			},
			Rules: netv1.IngressRuleArray{
				&netv1.IngressRuleArgs{
					Host: pulumi.String(args.Hostname),
					Http: &netv1.HTTPIngressRuleValueArgs{
						Paths: netv1.HTTPIngressPathArray{
							&netv1.HTTPIngressPathArgs{
								Path:     pulumi.String("/"),
								PathType: pulumi.String("Prefix"),
								Backend: &netv1.IngressBackendArgs{
									Service: &netv1.IngressServiceBackendArgs{
										Name: svc.Metadata.Name().Elem(),
										Port: &netv1.ServiceBackendPortArgs{
											Number: pulumi.Int(8080),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}, ingOpts...)
	if err != nil {
		return nil, err
	}

	return &Result{Ingress: ingress}, nil
}

// env is a small helper for plain (non-secret) environment variables.
func env(name, value string) corev1.EnvVarInput {
	return &corev1.EnvVarArgs{
		Name:  pulumi.String(name),
		Value: pulumi.String(value),
	}
}
