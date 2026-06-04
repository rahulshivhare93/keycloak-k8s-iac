// Package database deploys a hardened, internal-only PostgreSQL instance that
// backs Keycloak. It is never exposed outside the cluster.
package database

import (
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Args configures the PostgreSQL deployment.
type Args struct {
	Namespace string
	Image     string
	DBName    string
	Username  string
	Password  pulumi.StringInput
}

// Result references the PostgreSQL service for consumers (Keycloak).
type Result struct {
	ServiceName string
	Resource    pulumi.Resource
}

const appName = "postgres"

// Deploy creates the credentials secret, persistent volume claim, Deployment and
// ClusterIP Service for PostgreSQL.
func Deploy(ctx *pulumi.Context, args Args, opts ...pulumi.ResourceOption) (*Result, error) {
	labels := pulumi.StringMap{
		"app":                       pulumi.String(appName),
		"app.kubernetes.io/name":    pulumi.String(appName),
		"app.kubernetes.io/part-of": pulumi.String("keycloak"),
	}

	secret, err := corev1.NewSecret(ctx, "postgres-credentials", &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("postgres-credentials"),
			Namespace: pulumi.String(args.Namespace),
			Labels:    labels,
		},
		Type: pulumi.String("Opaque"),
		StringData: pulumi.StringMap{
			"POSTGRES_DB":       pulumi.String(args.DBName),
			"POSTGRES_USER":     pulumi.String(args.Username),
			"POSTGRES_PASSWORD": args.Password,
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	pvc, err := corev1.NewPersistentVolumeClaim(ctx, "postgres-data", &corev1.PersistentVolumeClaimArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("postgres-data"),
			Namespace: pulumi.String(args.Namespace),
			Labels:    labels,
		},
		Spec: &corev1.PersistentVolumeClaimSpecArgs{
			AccessModes: pulumi.StringArray{pulumi.String("ReadWriteOnce")},
			Resources: &corev1.VolumeResourceRequirementsArgs{
				Requests: pulumi.StringMap{"storage": pulumi.String("2Gi")},
			},
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	depOpts := append([]pulumi.ResourceOption{
		pulumi.DependsOn([]pulumi.Resource{secret, pvc}),
	}, opts...)

	_, err = appsv1.NewDeployment(ctx, "postgres", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(appName),
			Namespace: pulumi.String(args.Namespace),
			Labels:    labels,
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			// PVC is RWO, so never run two pods at once.
			Strategy: &appsv1.DeploymentStrategyArgs{Type: pulumi.String("Recreate")},
			Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec: &corev1.PodSpecArgs{
					AutomountServiceAccountToken: pulumi.Bool(false),
					SecurityContext: &corev1.PodSecurityContextArgs{
						RunAsNonRoot: pulumi.Bool(true),
						RunAsUser:    pulumi.Int(999),
						RunAsGroup:   pulumi.Int(999),
						FsGroup:      pulumi.Int(999),
						SeccompProfile: &corev1.SeccompProfileArgs{
							Type: pulumi.String("RuntimeDefault"),
						},
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String(appName),
							Image: pulumi.String(args.Image),
							EnvFrom: corev1.EnvFromSourceArray{
								&corev1.EnvFromSourceArgs{
									SecretRef: &corev1.SecretEnvSourceArgs{
										Name: secret.Metadata.Name().Elem(),
									},
								},
							},
							Env: corev1.EnvVarArray{
								// initdb refuses to use the volume root as PGDATA.
								&corev1.EnvVarArgs{
									Name:  pulumi.String("PGDATA"),
									Value: pulumi.String("/var/lib/postgresql/data/pgdata"),
								},
							},
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									Name:          pulumi.String("postgres"),
									ContainerPort: pulumi.Int(5432),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("256Mi"),
								},
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("500m"),
									"memory": pulumi.String("512Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContextArgs{
								AllowPrivilegeEscalation: pulumi.Bool(false),
								RunAsNonRoot:             pulumi.Bool(true),
								Capabilities: &corev1.CapabilitiesArgs{
									Drop: pulumi.StringArray{pulumi.String("ALL")},
								},
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("data"),
									MountPath: pulumi.String("/var/lib/postgresql/data"),
								},
							},
							ReadinessProbe: &corev1.ProbeArgs{
								Exec: &corev1.ExecActionArgs{
									Command: pulumi.StringArray{
										pulumi.String("pg_isready"),
										pulumi.String("-U"),
										pulumi.String(args.Username),
										pulumi.String("-d"),
										pulumi.String(args.DBName),
									},
								},
								InitialDelaySeconds: pulumi.Int(10),
								PeriodSeconds:       pulumi.Int(10),
								FailureThreshold:    pulumi.Int(6),
							},
							LivenessProbe: &corev1.ProbeArgs{
								Exec: &corev1.ExecActionArgs{
									Command: pulumi.StringArray{
										pulumi.String("pg_isready"),
										pulumi.String("-U"),
										pulumi.String(args.Username),
									},
								},
								InitialDelaySeconds: pulumi.Int(30),
								PeriodSeconds:       pulumi.Int(15),
								FailureThreshold:    pulumi.Int(6),
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: pulumi.String("data"),
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSourceArgs{
								ClaimName: pvc.Metadata.Name().Elem(),
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

	svc, err := corev1.NewService(ctx, "postgres", &corev1.ServiceArgs{
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
					Name:       pulumi.String("postgres"),
					Port:       pulumi.Int(5432),
					TargetPort: pulumi.Int(5432),
				},
			},
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	return &Result{
		ServiceName: appName,
		Resource:    svc,
	}, nil
}
