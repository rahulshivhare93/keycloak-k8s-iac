// Package security applies a default-deny NetworkPolicy posture to the namespace
// and then opens only the minimal flows required:
//   - all pods may resolve DNS (kube-dns in kube-system)
//   - Keycloak may receive traffic only from the ingress controller (kube-system)
//   - Keycloak may reach PostgreSQL on 5432
//   - PostgreSQL may receive traffic only from Keycloak
package security

import (
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Args configures network hardening.
type Args struct {
	Namespace string
}

func kubeSystemSelector() *metav1.LabelSelectorArgs {
	return &metav1.LabelSelectorArgs{
		MatchLabels: pulumi.StringMap{
			"kubernetes.io/metadata.name": pulumi.String("kube-system"),
		},
	}
}

func appSelector(app string) *metav1.LabelSelectorArgs {
	return &metav1.LabelSelectorArgs{
		MatchLabels: pulumi.StringMap{"app": pulumi.String(app)},
	}
}

// ApplyNetworkPolicies installs the default-deny baseline plus the minimal allow
// rules for Keycloak and PostgreSQL.
func ApplyNetworkPolicies(ctx *pulumi.Context, args Args, opts ...pulumi.ResourceOption) error {
	ns := pulumi.String(args.Namespace)

	// 1. Default deny all ingress and egress for every pod in the namespace.
	if _, err := netv1.NewNetworkPolicy(ctx, "default-deny", &netv1.NetworkPolicyArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("default-deny"), Namespace: ns},
		Spec: &netv1.NetworkPolicySpecArgs{
			PodSelector: &metav1.LabelSelectorArgs{},
			PolicyTypes: pulumi.StringArray{pulumi.String("Ingress"), pulumi.String("Egress")},
		},
	}, opts...); err != nil {
		return err
	}

	// 2. Allow every pod to resolve DNS via kube-dns.
	if _, err := netv1.NewNetworkPolicy(ctx, "allow-dns", &netv1.NetworkPolicyArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("allow-dns"), Namespace: ns},
		Spec: &netv1.NetworkPolicySpecArgs{
			PodSelector: &metav1.LabelSelectorArgs{},
			PolicyTypes: pulumi.StringArray{pulumi.String("Egress")},
			Egress: netv1.NetworkPolicyEgressRuleArray{
				&netv1.NetworkPolicyEgressRuleArgs{
					To: netv1.NetworkPolicyPeerArray{
						&netv1.NetworkPolicyPeerArgs{NamespaceSelector: kubeSystemSelector()},
					},
					Ports: netv1.NetworkPolicyPortArray{
						&netv1.NetworkPolicyPortArgs{Protocol: pulumi.String("UDP"), Port: pulumi.Int(53)},
						&netv1.NetworkPolicyPortArgs{Protocol: pulumi.String("TCP"), Port: pulumi.Int(53)},
					},
				},
			},
		},
	}, opts...); err != nil {
		return err
	}

	// 3. Keycloak: accept traffic only from the ingress controller (kube-system).
	if _, err := netv1.NewNetworkPolicy(ctx, "keycloak-ingress", &netv1.NetworkPolicyArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("keycloak-allow-ingress"), Namespace: ns},
		Spec: &netv1.NetworkPolicySpecArgs{
			PodSelector: appSelector("keycloak"),
			PolicyTypes: pulumi.StringArray{pulumi.String("Ingress")},
			Ingress: netv1.NetworkPolicyIngressRuleArray{
				&netv1.NetworkPolicyIngressRuleArgs{
					From: netv1.NetworkPolicyPeerArray{
						&netv1.NetworkPolicyPeerArgs{NamespaceSelector: kubeSystemSelector()},
					},
					Ports: netv1.NetworkPolicyPortArray{
						&netv1.NetworkPolicyPortArgs{Protocol: pulumi.String("TCP"), Port: pulumi.Int(8080)},
						&netv1.NetworkPolicyPortArgs{Protocol: pulumi.String("TCP"), Port: pulumi.Int(9000)},
					},
				},
			},
		},
	}, opts...); err != nil {
		return err
	}

	// 4. Keycloak: allow egress only to PostgreSQL on 5432 (DNS covered above).
	if _, err := netv1.NewNetworkPolicy(ctx, "keycloak-egress-db", &netv1.NetworkPolicyArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("keycloak-allow-egress-db"), Namespace: ns},
		Spec: &netv1.NetworkPolicySpecArgs{
			PodSelector: appSelector("keycloak"),
			PolicyTypes: pulumi.StringArray{pulumi.String("Egress")},
			Egress: netv1.NetworkPolicyEgressRuleArray{
				&netv1.NetworkPolicyEgressRuleArgs{
					To: netv1.NetworkPolicyPeerArray{
						&netv1.NetworkPolicyPeerArgs{PodSelector: appSelector("postgres")},
					},
					Ports: netv1.NetworkPolicyPortArray{
						&netv1.NetworkPolicyPortArgs{Protocol: pulumi.String("TCP"), Port: pulumi.Int(5432)},
					},
				},
			},
		},
	}, opts...); err != nil {
		return err
	}

	// 5. PostgreSQL: accept traffic only from Keycloak on 5432.
	if _, err := netv1.NewNetworkPolicy(ctx, "postgres-ingress", &netv1.NetworkPolicyArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("postgres-allow-ingress"), Namespace: ns},
		Spec: &netv1.NetworkPolicySpecArgs{
			PodSelector: appSelector("postgres"),
			PolicyTypes: pulumi.StringArray{pulumi.String("Ingress")},
			Ingress: netv1.NetworkPolicyIngressRuleArray{
				&netv1.NetworkPolicyIngressRuleArgs{
					From: netv1.NetworkPolicyPeerArray{
						&netv1.NetworkPolicyPeerArgs{PodSelector: appSelector("keycloak")},
					},
					Ports: netv1.NetworkPolicyPortArray{
						&netv1.NetworkPolicyPortArgs{Protocol: pulumi.String("TCP"), Port: pulumi.Int(5432)},
					},
				},
			},
		},
	}, opts...); err != nil {
		return err
	}

	return nil
}
