# Kubeconfig Provider Example

This example demonstrates how to use the kubeconfig provider to manage multiple Kubernetes clusters using kubeconfig secrets.

## Overview

The kubeconfig provider allows you to:
1. Discover and connect to multiple Kubernetes clusters using kubeconfig secrets
2. Run controllers that can operate across all discovered clusters
3. Manage cluster access through RBAC rules and service accounts

## Directory Structure

```
examples/kubeconfig/
├── rbac/                    # RBAC configuration templates
│   ├── serviceaccount.yaml  # Service account template
│   ├── clusterrole.yaml     # Cluster role template
│   └── clusterrolebinding.yaml
├── scripts/                 # Utility scripts
│   └── create-kubeconfig-secret.sh
└── main.go                 # Example operator implementation
```

## Usage

### 1. Setting Up Cluster Access

Use the `create-kubeconfig-secret.sh` script to create a kubeconfig secret for each cluster you want to manage:

```bash
./scripts/create-kubeconfig-secret.sh \
  -n cluster1 \
  -s my-operator-namespace \
  -k ~/.kube/config \
  -c prod-cluster
```

This will:
- Create a service account in the specified namespace
- Create RBAC rules for the service account
- Generate a kubeconfig using the service account token
- Store the kubeconfig in a secret

### 2. Customizing RBAC Rules

Edit the RBAC templates in the `rbac/` directory to define the permissions your operator needs:

```yaml
# rbac/clusterrole.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ${SECRET_NAME}-role
rules:
# Add permissions for your operator <--------------------------------
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["list", "get", "watch"]  # watch is needed for controllers that observe resources
```

Important RBAC considerations:
- Use `watch` verb if your controller needs to observe resource changes
- Use `list` and `get` for reading resources
- Use `create`, `update`, `patch`, `delete` for modifying resources
- Consider using `Role` instead of `ClusterRole` if you only need namespace-scoped permissions

### 3. Implementing Your Operator

Add your controllers to `main.go`:

```go
func main() {
    // Import your controllers here <--------------------------------
	"sigs.k8s.io/multicluster-runtime/examples/kubeconfig/controllers"

    //...

    // Run your controllers here <--------------------------------
	podWatcher := controllers.NewPodWatcher(mgr)
	if err := mgr.Add(podWatcher); err != nil {
		entryLog.Error(err, "Unable to add pod watcher")
		os.Exit(1)
	}
}
```

Your controllers can then use the manager to access any cluster and view the resources that the RBAC permissions allow.

## How It Works

1. The kubeconfig provider watches for secrets with a specific label in a namespace
2. When a new secret is found, it:
   - Extracts the kubeconfig data
   - Creates a new controller-runtime cluster
   - Makes the cluster available to your controllers
3. Your controllers can access any cluster through the manager
4. RBAC rules ensure your operator has the necessary permissions in each cluster

## Labels and Configuration

The provider uses the following labels and keys by default:
- Label: `sigs.k8s.io/multicluster-runtime-kubeconfig: "true"`
- Secret data key: `kubeconfig`

You can customize these in the provider options when creating it. 