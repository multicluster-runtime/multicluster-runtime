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

package manager

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"

	mccontext "github.com/multicluster-runtime/multicluster-runtime/pkg/context"

	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/multicluster-runtime/multicluster-runtime/pkg/multicluster"
)

// Manager is like crossplane-manager, without the cluster.Cluster interface.
var _ manager.Manager = &probe{}

type probe struct {
	Manager
	cluster.Cluster
}

// Add adds a runnable.
func (p *probe) Add(_ manager.Runnable) error {
	return nil
}

// Start starts the manager.
func (p *probe) Start(_ context.Context) error {
	return nil
}

// Manager is a multi-cluster-aware manager, like the controller-runtime Cluster,
// but without the direct cluster.Cluster methods.
type Manager interface {
	// Add will set requested dependencies on the component, and cause the component to be
	// started when Start is called.
	// Depending on if a Runnable implements LeaderElectionRunnable interface, a Runnable can be run in either
	// non-leaderelection mode (always running) or leader election mode (managed by leader election if enabled).
	Add(Runnable) error

	// Elected is closed when this manager is elected leader of a group of
	// managers, either because it won a leader election or because no leader
	// election was configured.
	Elected() <-chan struct{}

	// AddMetricsServerExtraHandler adds an extra handler served on path to the http server that serves metrics.
	// Might be useful to register some diagnostic endpoints e.g. pprof.
	//
	// Note that these endpoints are meant to be sensitive and shouldn't be exposed publicly.
	//
	// If the simple path -> handler mapping offered here is not enough,
	// a new http server/listener should be added as Runnable to the manager via Add method.
	AddMetricsServerExtraHandler(path string, handler http.Handler) error

	// AddHealthzCheck allows you to add Healthz checker
	AddHealthzCheck(name string, check healthz.Checker) error

	// AddReadyzCheck allows you to add Readyz checker
	AddReadyzCheck(name string, check healthz.Checker) error

	// Start starts all registered Controllers and blocks until the context is cancelled.
	// Returns an error if there is an error starting any controller.
	//
	// If LeaderElection is used, the binary must be exited immediately after this returns,
	// otherwise components that need leader election might continue to run after the leader
	// lock was lost.
	Start(ctx context.Context) error

	// GetWebhookServer returns a webhook.Server
	GetWebhookServer() webhook.Server

	// GetLogger returns this manager's logger.
	GetLogger() logr.Logger

	// GetControllerOptions returns controller global configuration options.
	GetControllerOptions() config.Controller

	// GetCluster returns a cluster for the given identifying cluster name. Get
	// returns an existing cluster if it has been created before.
	// If no cluster is known to the provider under the given cluster name,
	// an error should be returned.
	GetCluster(ctx context.Context, clusterName string) (cluster.Cluster, error)

	// ClusterFromContext returns the default cluster set in the context.
	ClusterFromContext(ctx context.Context) (cluster.Cluster, error)

	// GetLocalManager returns the underlying controller-runtime manager of the host.
	GetLocalManager() manager.Manager

	// GetProvider returns the multicluster provider, or nil if it is not set.
	GetProvider() multicluster.Provider

	multicluster.Aware
}

// Runnable allows a component to be started.
// It's very important that Start blocks until
// it's done running.
type Runnable interface {
	manager.Runnable
	multicluster.Aware
}

var _ Manager = &mcManager{}

type mcManager struct {
	manager.Manager
	provider multicluster.Provider

	mcRunnables []multicluster.Aware
}

// New returns a new Manager for creating Controllers.
func New(config *rest.Config, provider multicluster.Provider, opts manager.Options) (Manager, error) {
	mgr, err := manager.New(config, opts)
	if err != nil {
		return nil, err
	}
	return WithMultiCluster(mgr, provider)
}

// WithMultiCluster wraps a host manager to run multi-cluster controllers.
func WithMultiCluster(mgr manager.Manager, provider multicluster.Provider) (Manager, error) {
	return &mcManager{
		Manager:  mgr,
		provider: provider,
	}, nil
}

// GetCluster returns a cluster for the given identifying cluster name. Get
// returns an existing cluster if it has been created before.
// If no cluster is known to the provider under the given cluster name,
// an error should be returned.
func (m *mcManager) GetCluster(ctx context.Context, clusterName string) (cluster.Cluster, error) {
	if clusterName == "" {
		return m.Manager, nil
	}
	if m.provider == nil {
		return nil, fmt.Errorf("no multicluster provider set, but cluster %q passed", clusterName)
	}
	return m.provider.Get(ctx, clusterName)
}

// ClusterFromContext returns the default cluster set in the context.
func (m *mcManager) ClusterFromContext(ctx context.Context) (cluster.Cluster, error) {
	clusterName, ok := mccontext.ClusterFrom(ctx)
	if !ok {
		return nil, fmt.Errorf("no cluster set in context, use ReconcilerWithCluster helper when building the controller")
	}
	return m.GetCluster(ctx, clusterName)
}

// GetLocalManager returns the underlying controller-runtime manager of the host.
func (m *mcManager) GetLocalManager() manager.Manager {
	return m.Manager
}

// GetProvider returns the multicluster provider, or nil if it is not set.
func (m *mcManager) GetProvider() multicluster.Provider {
	return m.provider
}

// Add will set requested dependencies on the component, and cause the component to be
// started when Start is called.
func (m *mcManager) Add(r Runnable) (err error) {
	m.mcRunnables = append(m.mcRunnables, r)
	defer func() {
		if err != nil {
			m.mcRunnables = m.mcRunnables[:len(m.mcRunnables)-1]
		}
	}()

	return m.Manager.Add(r)
}

// Engage gets called when the component should start operations for the given
// Cluster. ctx is cancelled when the cluster is disengaged.
func (m *mcManager) Engage(ctx context.Context, name string, cl cluster.Cluster) error {
	ctx, cancel := context.WithCancel(ctx)
	for _, r := range m.mcRunnables {
		if err := r.Engage(ctx, name, cl); err != nil {
			cancel()
			return fmt.Errorf("failed to engage cluster %q: %w", name, err)
		}
	}
	return nil
}
