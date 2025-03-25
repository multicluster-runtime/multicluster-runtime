/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package kubeconfig provides a Kubernetes cluster provider that watches secrets
// containing kubeconfig data and creates controller-runtime clusters for each.
package kubeconfig

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// ClusterReconciler defines the operations required from a reconciler for cluster operations
type ClusterReconciler interface {
	// RegisterCluster registers a new cluster with the reconciler
	RegisterCluster(name string, cl cluster.Cluster)

	// UnregisterCluster removes a cluster from the reconciler
	UnregisterCluster(name string)

	// GetCluster returns a cluster for the given name
	GetCluster(ctx context.Context, name string) (cluster.Cluster, error)

	// ListClusters returns a list of all registered clusters
	ListClusters() map[string]cluster.Cluster

	// ListClustersWithLog returns a list of all registered clusters and logs them
	ListClustersWithLog() map[string]cluster.Cluster
}

// KubeClusterManager defines an interface for managing multiple clusters via kubeconfig
type KubeClusterManager interface {
	manager.Manager

	// GetCluster returns a cluster for the given name
	GetCluster(ctx context.Context, name string) (cluster.Cluster, error)

	// Engage registers a new cluster with the manager
	Engage(ctx context.Context, name string, cl cluster.Cluster) error

	// Disengage removes a cluster from the manager
	Disengage(ctx context.Context, name string) error

	// ListClusters returns a list of all registered clusters
	ListClusters() map[string]cluster.Cluster
}

// Provider defines an interface for a kubecofnig-based cluster provider
type Provider interface {
	// Get returns a cluster by name
	Get(ctx context.Context, name string) (cluster.Cluster, error)

	// Run starts the provider
	Run(ctx context.Context, mgr KubeClusterManager) error

	// IndexField indexes a field on all clusters
	IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error
}
