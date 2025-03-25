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

	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var setupLog = log.Log.WithName("kubeconfig-provider")

// KubeconfigClusterManager is an implementation of the KubeClusterManager interface that manages
// multiple Kubernetes clusters using kubeconfig.
type KubeconfigClusterManager struct {
	manager.Manager
	clusters   map[string]cluster.Cluster
	reconciler ClusterReconciler
	mu         sync.RWMutex
}

// Ensure KubeconfigClusterManager implements the KubeClusterManager interface
var _ KubeClusterManager = &KubeconfigClusterManager{}

// NewKubeconfigClusterManager creates a new KubeconfigClusterManager.
func NewKubeconfigClusterManager(mgr manager.Manager, reconciler ClusterReconciler) *KubeconfigClusterManager {
	return &KubeconfigClusterManager{
		Manager:    mgr,
		clusters:   make(map[string]cluster.Cluster),
		reconciler: reconciler,
	}
}

// GetCluster returns a cluster by name.
func (a *KubeconfigClusterManager) GetCluster(ctx context.Context, name string) (cluster.Cluster, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	c, ok := a.clusters[name]
	if !ok {
		return nil, fmt.Errorf("cluster %s not found", name)
	}
	return c, nil
}

// Engage adds a cluster to the manager.
func (a *KubeconfigClusterManager) Engage(ctx context.Context, name string, c cluster.Cluster) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clusters[name] = c
	a.reconciler.RegisterCluster(name, c)
	return nil
}

// Disengage removes a cluster from the manager.
func (a *KubeconfigClusterManager) Disengage(ctx context.Context, name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reconciler.UnregisterCluster(name)
	delete(a.clusters, name)
	return nil
}

// ListClusters returns a list of all registered clusters
func (a *KubeconfigClusterManager) ListClusters() map[string]cluster.Cluster {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.clusters == nil {
		return map[string]cluster.Cluster{}
	}

	// Return a copy of the map to prevent concurrent access
	clusters := make(map[string]cluster.Cluster, len(a.clusters))
	for name, cl := range a.clusters {
		clusters[name] = cl
	}
	return clusters
}

// ListClustersWithLog returns a list of all registered clusters and logs them
func (a *KubeconfigClusterManager) ListClustersWithLog() map[string]cluster.Cluster {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.clusters == nil {
		setupLog.Info("No clusters registered")
		return map[string]cluster.Cluster{}
	}

	// Log all clusters
	clusterNames := make([]string, 0, len(a.clusters))
	for name := range a.clusters {
		clusterNames = append(clusterNames, name)
	}
	setupLog.Info("Current registered clusters",
		"totalClusters", len(a.clusters),
		"clusterNames", strings.Join(clusterNames, ", "))

	// Return a copy of the map to prevent concurrent access
	clusters := make(map[string]cluster.Cluster, len(a.clusters))
	for name, cl := range a.clusters {
		clusters[name] = cl
	}
	return clusters
}
