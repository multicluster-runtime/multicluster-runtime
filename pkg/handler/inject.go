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

package handler

import (
	"context"
	"time"

	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

// TypedInjectCluster wraps a handler.TypedEventHandler with a cluster-aware
// request type and injects the cluster name. In contrast to TypeWithCluster,
// this function does not lift the type to cluster-awareness.
func TypedInjectCluster[object client.Object, request mcreconcile.ClusterAware[request]](h handler.TypedEventHandler[object, request]) TypedEventHandlerFunc[object, request] {
	return func(clusterName string, cl cluster.Cluster) handler.TypedEventHandler[object, request] {
		return &clusterInjectingHandler[object, request]{h: h, clusterName: clusterName}
	}
}

var _ handler.TypedEventHandler[client.Object, mcreconcile.Request] = &clusterHandler[client.Object]{}

type clusterInjectingHandler[object client.Object, request mcreconcile.ClusterAware[request]] struct {
	h           handler.TypedEventHandler[object, request]
	clusterName string
}

// Create implements EventHandler.
func (e *clusterInjectingHandler[object, request]) Create(ctx context.Context, evt event.TypedCreateEvent[object], q workqueue.TypedRateLimitingInterface[request]) {
	e.h.Create(ctx, evt, clusterInjectingQueue[request]{q: q, cl: e.clusterName})
}

// Update implements EventHandler.
func (e *clusterInjectingHandler[object, request]) Update(ctx context.Context, evt event.TypedUpdateEvent[object], q workqueue.TypedRateLimitingInterface[request]) {
	e.h.Update(ctx, evt, clusterInjectingQueue[request]{q: q, cl: e.clusterName})
}

// Delete implements EventHandler.
func (e *clusterInjectingHandler[object, request]) Delete(ctx context.Context, evt event.TypedDeleteEvent[object], q workqueue.TypedRateLimitingInterface[request]) {
	e.h.Delete(ctx, evt, clusterInjectingQueue[request]{q: q, cl: e.clusterName})
}

// Generic implements EventHandler.
func (e *clusterInjectingHandler[object, request]) Generic(ctx context.Context, evt event.TypedGenericEvent[object], q workqueue.TypedRateLimitingInterface[request]) {
	e.h.Generic(ctx, evt, clusterInjectingQueue[request]{q: q, cl: e.clusterName})
}

var _ workqueue.TypedRateLimitingInterface[mcreconcile.Request] = &clusterInjectingQueue[mcreconcile.Request]{}

type clusterInjectingQueue[request mcreconcile.ClusterAware[request]] struct {
	q  workqueue.TypedRateLimitingInterface[request]
	cl string
}

func (c clusterInjectingQueue[request]) Add(item request) {
	c.q.Add(item.WithCluster(c.cl))
}

func (c clusterInjectingQueue[request]) Len() int {
	return c.q.Len()
}

func (c clusterInjectingQueue[request]) Get() (item request, shutdown bool) {
	it, shutdown := c.q.Get()
	return it.WithCluster(""), shutdown
}

func (c clusterInjectingQueue[request]) Done(item request) {
	c.q.Done(item.WithCluster(c.cl))
}

func (c clusterInjectingQueue[request]) ShutDown() {
	c.q.ShutDown()
}

func (c clusterInjectingQueue[request]) ShutDownWithDrain() {
	c.q.ShutDownWithDrain()
}

func (c clusterInjectingQueue[request]) ShuttingDown() bool {
	return c.q.ShuttingDown()
}

func (c clusterInjectingQueue[request]) AddAfter(item request, duration time.Duration) {
	c.q.AddAfter(item.WithCluster(c.cl), duration)
}

func (c clusterInjectingQueue[request]) AddRateLimited(item request) {
	c.q.AddRateLimited(item.WithCluster(c.cl))
}

func (c clusterInjectingQueue[request]) Forget(item request) {
	c.q.Forget(item.WithCluster(c.cl))
}

func (c clusterInjectingQueue[request]) NumRequeues(item request) int {
	return c.q.NumRequeues(item.WithCluster(c.cl))
}
