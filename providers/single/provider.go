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

package namespace

import (
	"context"
	"fmt"

	mcmanager "github.com/multicluster-runtime/multicluster-runtime/pkg/manager"
	"github.com/multicluster-runtime/multicluster-runtime/pkg/multicluster"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

var _ multicluster.Provider = &Provider{}

// Provider is a provider that engages the passed cluster.
type Provider struct {
	name string
	cl   cluster.Cluster
}

// New returns a provider engaging the passed cluster under the given name.
// It does not start the passed cluster.
func New(name string, cl cluster.Cluster) *Provider {
	return &Provider{
		name: name,
		cl:   cl,
	}
}

// Run starts the provider and blocks.
func (p *Provider) Run(ctx context.Context, _ mcmanager.Manager) error {
	<-ctx.Done()
	return nil
}

// Get returns the cluster with the given name.
func (p *Provider) Get(_ context.Context, clusterName string) (cluster.Cluster, error) {
	if clusterName == p.name {
		return p.cl, nil
	}
	return nil, fmt.Errorf("cluster %s not found", clusterName)
}
