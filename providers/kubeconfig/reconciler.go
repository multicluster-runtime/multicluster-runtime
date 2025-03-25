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

package kubeconfig

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// MulticlusterReconciler handles reconciliations across multiple clusters
type MulticlusterReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	lock     sync.RWMutex
	clusters map[string]cluster.Cluster
}

// Ensure MulticlusterReconciler implements ClusterReconciler
var _ ClusterReconciler = &MulticlusterReconciler{}

// NewMulticlusterReconciler creates a new MulticlusterReconciler
func NewMulticlusterReconciler(c client.Client, scheme *runtime.Scheme) *MulticlusterReconciler {
	return &MulticlusterReconciler{
		Client:   c,
		Scheme:   scheme,
		clusters: make(map[string]cluster.Cluster),
	}
}

// RegisterCluster registers a new cluster with the reconciler
func (r *MulticlusterReconciler) RegisterCluster(name string, cl cluster.Cluster) {
	r.lock.Lock()
	defer r.lock.Unlock()

	setupLog.Info("Registering cluster with reconciler", "name", name)

	if r.clusters == nil {
		r.clusters = make(map[string]cluster.Cluster)
	}

	// Store the cluster
	r.clusters[name] = cl
	setupLog.Info("Registered cluster with reconciler", "name", name, "totalClusters", len(r.clusters))

	// Log all clusters after registration
	clusterNames := make([]string, 0, len(r.clusters))
	for name := range r.clusters {
		clusterNames = append(clusterNames, name)
	}
	setupLog.Info("Current clusters after registration",
		"totalClusters", len(r.clusters),
		"clusterNames", strings.Join(clusterNames, ", "))
}

// UnregisterCluster removes a cluster from the reconciler
func (r *MulticlusterReconciler) UnregisterCluster(name string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.clusters, name)
	setupLog.Info("Unregistered cluster", "name", name)
}

// GetCluster returns a cluster for the given name
func (r *MulticlusterReconciler) GetCluster(ctx context.Context, name string) (cluster.Cluster, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if cl, ok := r.clusters[name]; ok {
		return cl, nil
	}
	return nil, fmt.Errorf("cluster %s not found", name)
}

// ListClusters returns a list of all registered clusters
func (r *MulticlusterReconciler) ListClusters() map[string]cluster.Cluster {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if r.clusters == nil {
		return map[string]cluster.Cluster{}
	}

	// Return a copy of the map to prevent concurrent access
	clusters := make(map[string]cluster.Cluster, len(r.clusters))
	for name, cl := range r.clusters {
		clusters[name] = cl
	}
	return clusters
}

// ListClustersWithLog returns a list of all registered clusters and logs them
func (r *MulticlusterReconciler) ListClustersWithLog() map[string]cluster.Cluster {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if r.clusters == nil {
		setupLog.Info("No clusters registered")
		return map[string]cluster.Cluster{}
	}

	// Log all clusters
	clusterNames := make([]string, 0, len(r.clusters))
	for name := range r.clusters {
		clusterNames = append(clusterNames, name)
	}
	setupLog.Info("Current registered clusters",
		"totalClusters", len(r.clusters),
		"clusterNames", strings.Join(clusterNames, ", "))

	// Return a copy of the map to prevent concurrent access
	clusters := make(map[string]cluster.Cluster, len(r.clusters))
	for name, cl := range r.clusters {
		clusters[name] = cl
	}
	return clusters
}
