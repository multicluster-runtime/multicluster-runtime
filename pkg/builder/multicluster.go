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

package builder

import (
	"context"
	"time"

	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// handlerWithCluster wraps a handler and injects the cluster name into the
// reuqests that are enqueued.
func handlerWithCluster[object any, request mcreconcile.ClusterAware[request]](name string, h handler.TypedEventHandler[object, request]) handler.TypedEventHandler[object, request] {
	return handler.TypedFuncs[object, request]{
		CreateFunc: func(ctx context.Context, e event.TypedCreateEvent[object], q workqueue.TypedRateLimitingInterface[request]) {
			h.Create(ctx, e, clusterAwareWorkqueue[request]{cluster: name, q: q})
		},
	}
}

type clusterAwareWorkqueue[request mcreconcile.ClusterAware[request]] struct {
	cluster string
	q       workqueue.TypedRateLimitingInterface[request]
}

var _ workqueue.TypedInterface[mcreconcile.Request] = &clusterAwareWorkqueue[mcreconcile.Request]{}

func (q clusterAwareWorkqueue[request]) Add(item request) {
	q.q.Add(item.WithCluster(q.cluster))
}

func (q clusterAwareWorkqueue[request]) AddAfter(item request, duration time.Duration) {
	q.q.AddAfter(item.WithCluster(q.cluster), duration)
}

func (q clusterAwareWorkqueue[request]) AddRateLimited(item request) {
	q.q.AddRateLimited(item.WithCluster(q.cluster))
}

func (q clusterAwareWorkqueue[request]) Forget(item request) {
	q.q.Forget(item.WithCluster(q.cluster))
}

func (q clusterAwareWorkqueue[request]) NumRequeues(item request) int {
	return q.q.NumRequeues(item.WithCluster(q.cluster))
}

func (q clusterAwareWorkqueue[request]) Len() int {
	return q.q.Len()
}

func (q clusterAwareWorkqueue[request]) Get() (item request, shutdown bool) {
	return q.q.Get()
}

func (q clusterAwareWorkqueue[request]) Done(item request) {
	q.q.Done(item.WithCluster(q.cluster))
}

func (q clusterAwareWorkqueue[request]) ShutDown() {
	q.q.ShutDown()
}

func (q clusterAwareWorkqueue[request]) ShutDownWithDrain() {
	q.q.ShutDownWithDrain()
}

func (q clusterAwareWorkqueue[request]) ShuttingDown() bool {
	return q.q.ShuttingDown()
}
