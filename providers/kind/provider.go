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

package kind

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	mcmanager "github.com/multicluster-runtime/multicluster-runtime/pkg/manager"
	"github.com/multicluster-runtime/multicluster-runtime/pkg/multicluster"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kind "sigs.k8s.io/kind/pkg/cluster"
)

// New creates a new kind cluster Provider.
func New() *Provider {
	return &Provider{
		log:       log.Log.WithName("kind-cluster-Provider"),
		clusters:  map[string]cluster.Cluster{},
		cancelFns: map[string]context.CancelFunc{},
	}
}

// Provider is a cluster Provider that works with a local Kind instance.
type Provider struct {
	opts      []cluster.Option
	log       logr.Logger
	lock      sync.RWMutex
	clusters  map[string]cluster.Cluster
	cancelFns map[string]context.CancelFunc
}

var _ multicluster.Provider = &Provider{}

func (k *Provider) Get(ctx context.Context, clusterName string) (cluster.Cluster, error) {
	k.lock.RLock()
	defer k.lock.RUnlock()
	if cl, ok := k.clusters[clusterName]; ok {
		return cl, nil
	}

	return nil, fmt.Errorf("cluster %s not found", clusterName)
}

func (k *Provider) Run(ctx context.Context, mgr mcmanager.Manager) error {
	k.log.Info("Starting kind cluster Provider")

	provider := kind.NewProvider()

	// initial list to smoke test
	if _, err := provider.List(); err != nil {
		return err
	}

	return wait.PollUntilContextCancel(ctx, time.Second*2, true, func(ctx context.Context) (done bool, err error) {
		list, err := provider.List()
		if err != nil {
			k.log.Info("failed to list kind clusters", "error", err)
			return false, nil // keep going
		}

		// start new clusters
		for _, clusterName := range list {
			log := k.log.WithValues("cluster", clusterName)

			// skip?
			if !strings.HasPrefix(clusterName, "fleet-") {
				continue
			}
			k.lock.RLock()
			if _, ok := k.clusters[clusterName]; ok {
				k.lock.RUnlock()
				continue
			}
			k.lock.RUnlock()

			// create a new cluster
			kubeconfig, err := provider.KubeConfig(clusterName, false)
			if err != nil {
				k.log.Info("failed to get kind kubeconfig", "error", err)
				return false, nil // keep going
			}
			cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
			if err != nil {
				k.log.Info("failed to create rest config", "error", err)
				return false, nil // keep going
			}
			cl, err := cluster.New(cfg, k.opts...)
			if err != nil {
				k.log.Info("failed to create cluster", "error", err)
				return false, nil // keep going
			}
			clusterCtx, cancel := context.WithCancel(ctx)
			go func() {
				if err := cl.Start(clusterCtx); err != nil {
					log.Error(err, "failed to start cluster")
					return
				}
			}()
			if !cl.GetCache().WaitForCacheSync(ctx) {
				cancel()
				log.Info("failed to sync cache")
				return false, nil
			}

			// remember
			k.lock.Lock()
			k.clusters[clusterName] = cl
			k.cancelFns[clusterName] = cancel
			k.lock.Unlock()

			k.log.Info("Added new cluster", "cluster", clusterName)

			// engage manager
			if mgr != nil {
				if err := mgr.Engage(clusterCtx, clusterName, cl); err != nil {
					log.Error(err, "failed to engage manager")
					k.lock.Lock()
					delete(k.clusters, clusterName)
					delete(k.cancelFns, clusterName)
					k.lock.Unlock()
					return false, nil
				}
			}
		}

		// remove old clusters
		kindNames := sets.New(list...)
		k.lock.Lock()
		clusterNames := make([]string, 0, len(k.clusters))
		for name := range k.clusters {
			clusterNames = append(clusterNames, name)
		}
		k.lock.Unlock()
		for _, name := range clusterNames {
			if !kindNames.Has(name) {
				// stop and forget
				k.lock.Lock()
				k.cancelFns[name]()
				delete(k.clusters, name)
				delete(k.cancelFns, name)
				k.lock.Unlock()

				k.log.Info("Cluster removed", "cluster", name)
			}
		}

		return false, nil
	})
}
