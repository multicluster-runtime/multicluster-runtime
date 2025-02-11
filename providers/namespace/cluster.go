/*
Copyright 2024 The Kubernetes Authors.

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
	"time"

	apiruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// WithClusterNameIndex adds indexers for cluster name and namespace.
func WithClusterNameIndex() cluster.Option {
	return func(options *cluster.Options) {
		old := options.Cache.NewInformer
		options.Cache.NewInformer = func(watcher toolscache.ListerWatcher, object apiruntime.Object, duration time.Duration, indexers toolscache.Indexers) toolscache.SharedIndexInformer {
			var inf toolscache.SharedIndexInformer
			if old != nil {
				inf = old(watcher, object, duration, indexers)
			} else {
				inf = toolscache.NewSharedIndexInformer(watcher, object, duration, indexers)
			}
			if err := inf.AddIndexers(toolscache.Indexers{
				ClusterNameIndex: func(obj any) ([]string, error) {
					o := obj.(client.Object)
					return []string{
						fmt.Sprintf("%s/%s", o.GetNamespace(), o.GetName()),
						fmt.Sprintf("%s/%s", "*", o.GetName()),
					}, nil
				},
				ClusterIndex: func(obj any) ([]string, error) {
					o := obj.(client.Object)
					return []string{o.GetNamespace()}, nil
				},
			}); err != nil {
				utilruntime.HandleError(fmt.Errorf("unable to add cluster name indexers: %w", err))
			}
			return inf
		}
	}
}

type NamespacedCluster struct {
	clusterName string
	cluster.Cluster
}

func (c *NamespacedCluster) Name() string {
	return c.clusterName
}

func (c *NamespacedCluster) GetCache() cache.Cache {
	return &NamespacedCache{clusterName: c.clusterName, Cache: c.Cluster.GetCache()}
}

func (c *NamespacedCluster) GetClient() client.Client {
	return &NamespacedClient{clusterName: c.clusterName, Client: c.Cluster.GetClient()}
}

func (c *NamespacedCluster) GetEventRecorderFor(name string) record.EventRecorder {
	panic("implement me")
}

func (c *NamespacedCluster) GetAPIReader() client.Reader {
	return c.GetClient()
}

func (c *NamespacedCluster) Start(ctx context.Context) error {
	return nil // no-op as this is shared
}
