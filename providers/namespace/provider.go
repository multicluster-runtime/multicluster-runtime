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
	"sync"

	"github.com/go-logr/logr"

	mcmanager "github.com/multicluster-runtime/multicluster-runtime/pkg/manager"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// NamespacedClusterProvider is a cluster provider that represents each namespace
// as a dedicated cluster with only a "default" namespace. It maps each namespace
// to "default" and vice versa, simulating a multi-cluster setup. It uses one
// informer to watch objects for all namespaces.
type NamespacedClusterProvider struct {
	cluster cluster.Cluster

	mgr manager.Manager

	log       logr.Logger
	lock      sync.RWMutex
	clusters  map[string]cluster.Cluster
	cancelFns map[string]context.CancelFunc
}

func NewNamespacedClusterProvider(cl cluster.Cluster) *NamespacedClusterProvider {
	return &NamespacedClusterProvider{
		cluster:   cl,
		log:       log.Log.WithName("namespaced-cluster-provider"),
		clusters:  map[string]cluster.Cluster{},
		cancelFns: map[string]context.CancelFunc{},
	}
}

func (p *NamespacedClusterProvider) Start(ctx context.Context, mgr mcmanager.Manager) error {
	nsInf, err := p.cluster.GetCache().GetInformer(ctx, &corev1.Namespace{})
	if err != nil {
		return err
	}

	if _, err := nsInf.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ns := obj.(*corev1.Namespace)
			p.log.WithValues("namespace", ns.Name).Info("Encountered namespace")

			p.lock.RLock()
			_, ok := p.clusters[ns.Name]
			p.lock.RUnlock()

			if ok {
				return
			}

			// create new cluster
			p.lock.Lock()
			clusterCtx, cancel := context.WithCancel(ctx)
			cl := &NamespacedCluster{clusterName: ns.Name, Cluster: p.cluster}
			p.clusters[ns.Name] = cl
			p.cancelFns[ns.Name] = cancel
			p.lock.Unlock()

			if err := mgr.Engage(clusterCtx, ns.Name, cl); err != nil {
				utilruntime.HandleError(fmt.Errorf("failed to engage manager with cluster %q: %w", ns.Name, err))

				// cleanup
				p.lock.Lock()
				delete(p.clusters, ns.Name)
				delete(p.cancelFns, ns.Name)
				p.lock.Unlock()
			}
		},
		DeleteFunc: func(obj interface{}) {
			ns := obj.(*corev1.Namespace)

			p.lock.RLock()
			cancel, ok := p.cancelFns[ns.Name]
			if !ok {
				p.lock.RUnlock()
				return
			}
			p.lock.RUnlock()

			cancel()

			// stop and forget
			p.lock.Lock()
			p.cancelFns[ns.Name]()
			delete(p.clusters, ns.Name)
			delete(p.cancelFns, ns.Name)
			p.lock.Unlock()
		},
	}); err != nil {
		return err
	}

	return nil
}

func (p *NamespacedClusterProvider) Get(ctx context.Context, clusterName string) (cluster.Cluster, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	if cl, ok := p.clusters[clusterName]; ok {
		return cl, nil
	}
	return nil, fmt.Errorf("cluster %s not found", clusterName)
}
