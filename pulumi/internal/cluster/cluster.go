// Package cluster provisions a local k3d (k3s / Rancher) Kubernetes cluster
// using Pulumi's command provider, so the whole setup is reproducible from a
// single `pulumi up`.
package cluster

import (
	"fmt"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Result carries the kubeconfig of the provisioned cluster and the resource
// other resources should depend on to enforce correct ordering.
type Result struct {
	Kubeconfig pulumi.StringOutput
	Resource   pulumi.Resource
}

// Provision creates a k3d cluster named name, mapping the host port httpsPort to
// the cluster's HTTPS (443) load balancer, and returns its kubeconfig.
func Provision(ctx *pulumi.Context, name string, httpsPort int) (*Result, error) {
	createScript := fmt.Sprintf(`set -euo pipefail
if k3d cluster list %[1]q >/dev/null 2>&1; then
  echo "cluster %[1]s already exists; reusing"
else
  k3d cluster create %[1]q \
    --servers 1 \
    --agents 1 \
    --port "%[2]d:443@loadbalancer" \
    --k3s-arg "--disable=metrics-server@server:*" \
    --wait
fi`, name, httpsPort)

	deleteScript := fmt.Sprintf(`k3d cluster delete %q || true`, name)

	createCmd, err := local.NewCommand(ctx, "k3d-cluster", &local.CommandArgs{
		Create: pulumi.String(createScript),
		Delete: pulumi.String(deleteScript),
		Triggers: pulumi.Array{
			pulumi.String(name),
			pulumi.Int(httpsPort),
		},
	})
	if err != nil {
		return nil, err
	}

	// Fetch the kubeconfig only after the cluster exists. Re-runs on update so a
	// recreated cluster yields a fresh kubeconfig.
	kubeconfigCmd, err := local.NewCommand(ctx, "k3d-kubeconfig", &local.CommandArgs{
		Create: pulumi.String(fmt.Sprintf("k3d kubeconfig get %q", name)),
		Update: pulumi.String(fmt.Sprintf("k3d kubeconfig get %q", name)),
		Triggers: pulumi.Array{
			createCmd.ID(),
		},
	}, pulumi.DependsOn([]pulumi.Resource{createCmd}))
	if err != nil {
		return nil, err
	}

	return &Result{
		Kubeconfig: kubeconfigCmd.Stdout,
		Resource:   kubeconfigCmd,
	}, nil
}
