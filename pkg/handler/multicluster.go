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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
)

// EventHandlerFunc produces a handler.EventHandler for a cluster.
type EventHandlerFunc = func(string, cluster.Cluster) EventHandler

// TypedEventHandlerFunc produces a handler.TypedEventHandler for a cluster.
type TypedEventHandlerFunc[object client.Object, request comparable] = func(string, cluster.Cluster) handler.TypedEventHandler[object, mcreconcile.TypedRequest[request]]

// WithCluster wraps a handler.EventHandler without multi-cluster support to be
// compatible with multi-cluster by injecting the cluster name.
func WithCluster(h handler.EventHandler) EventHandlerFunc {
	return TypedWithCluster[client.Object, reconcile.Request](h)
}

// TypedWithCluster wraps a handler.TypedEventHandler without multi-cluster
// support to be compatible with multi-cluster by injecting the cluster name.
func TypedWithCluster[object client.Object, request comparable](h handler.TypedEventHandler[object, request]) TypedEventHandlerFunc[object, request] {
	return func(clusterName string, cl cluster.Cluster) handler.TypedEventHandler[object, mcreconcile.TypedRequest[request]] {
		return &clusterHandler[object, request]{h: h, clusterName: clusterName}
	}
}

var _ handler.TypedEventHandler[client.Object, mcreconcile.Request] = &clusterHandler[client.Object, reconcile.Request]{}

type clusterHandler[object client.Object, request comparable] struct {
	h           handler.TypedEventHandler[object, request]
	clusterName string
}

// Create implements EventHandler.
func (e *clusterHandler[object, request]) Create(ctx context.Context, evt event.TypedCreateEvent[object], q workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[request]]) {
	e.h.Create(ctx, evt, clusterQueue[request]{q: q, cl: e.clusterName})
}

// Update implements EventHandler.
func (e *clusterHandler[object, request]) Update(ctx context.Context, evt event.TypedUpdateEvent[object], q workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[request]]) {
	e.h.Update(ctx, evt, clusterQueue[request]{q: q, cl: e.clusterName})
}

// Delete implements EventHandler.
func (e *clusterHandler[object, request]) Delete(ctx context.Context, evt event.TypedDeleteEvent[object], q workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[request]]) {
	e.h.Delete(ctx, evt, clusterQueue[request]{q: q, cl: e.clusterName})
}

// Generic implements EventHandler.
func (e *clusterHandler[object, request]) Generic(ctx context.Context, evt event.TypedGenericEvent[object], q workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[request]]) {
	e.h.Generic(ctx, evt, clusterQueue[request]{q: q, cl: e.clusterName})
}

var _ workqueue.TypedRateLimitingInterface[reconcile.Request] = &clusterQueue[reconcile.Request]{}

type clusterQueue[request comparable] struct {
	q  workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[request]]
	cl string
}

func (c clusterQueue[request]) Add(item request) {
	c.q.Add(mcreconcile.TypedRequest[request]{
		Request:     item,
		ClusterName: c.cl,
	})
}

func (c clusterQueue[request]) Len() int {
	return c.q.Len()
}

func (c clusterQueue[request]) Get() (item request, shutdown bool) {
	req, shutdown := c.q.Get()
	return req.Request, shutdown
}

func (c clusterQueue[request]) Done(item request) {
	c.q.Done(mcreconcile.TypedRequest[request]{
		Request:     item,
		ClusterName: c.cl,
	})
}

func (c clusterQueue[request]) ShutDown() {
	c.q.ShutDown()
}

func (c clusterQueue[request]) ShutDownWithDrain() {
	c.q.ShutDownWithDrain()
}

func (c clusterQueue[request]) ShuttingDown() bool {
	return c.q.ShuttingDown()
}

func (c clusterQueue[request]) AddAfter(item request, duration time.Duration) {
	c.q.AddAfter(mcreconcile.TypedRequest[request]{
		Request:     item,
		ClusterName: c.cl,
	}, duration)
}

func (c clusterQueue[request]) AddRateLimited(item request) {
	c.q.AddRateLimited(mcreconcile.TypedRequest[request]{
		Request:     item,
		ClusterName: c.cl,
	})
}

func (c clusterQueue[request]) Forget(item request) {
	c.q.Forget(mcreconcile.TypedRequest[request]{
		Request:     item,
		ClusterName: c.cl,
	})
}

func (c clusterQueue[request]) NumRequeues(item request) int {
	return c.q.NumRequeues(mcreconcile.TypedRequest[request]{
		Request:     item,
		ClusterName: c.cl,
	})
}
