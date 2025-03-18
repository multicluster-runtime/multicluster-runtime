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

	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

// EventHandlerFunc produces a handler.EventHandler for a cluster.
type EventHandlerFunc = func(string, cluster.Cluster) EventHandler

// TypedEventHandlerFunc produces a handler.TypedEventHandler for a cluster.
type TypedEventHandlerFunc[object client.Object, request mcreconcile.ClusterAware[request]] func(string, cluster.Cluster) handler.TypedEventHandler[object, request]

// ForCluster wraps a handler.EventHandler without multi-cluster support for one
// concrete cluster.
func ForCluster(h handler.EventHandler, clusterName string) EventHandler {
	return &clusterHandler[client.Object]{h: h, clusterName: clusterName}
}

// TypedForCluster wraps a handler.TypedEventHandler without multi-cluster
// support for one concrete cluster.
func TypedForCluster[object client.Object](h handler.TypedEventHandler[object, reconcile.Request], clusterName string) handler.TypedEventHandler[object, mcreconcile.Request] {
	return &clusterHandler[object]{h: h, clusterName: clusterName}
}

// Lift wraps a handler.EventHandler without multi-cluster support to be
// compatible with multi-cluster by injecting the cluster name.
func Lift(h handler.EventHandler) EventHandlerFunc {
	return TypedLift[client.Object](h)
}

// TypedLift wraps a handler.TypedEventHandler without multi-cluster
// support to be compatible with multi-cluster by injecting the cluster name.
func TypedLift[object client.Object](h handler.TypedEventHandler[object, reconcile.Request]) TypedEventHandlerFunc[object, mcreconcile.Request] {
	return func(clusterName string, cl cluster.Cluster) handler.TypedEventHandler[object, mcreconcile.Request] {
		return &clusterHandler[object]{h: h, clusterName: clusterName}
	}
}

var _ handler.TypedEventHandler[client.Object, mcreconcile.Request] = &clusterHandler[client.Object]{}

type clusterHandler[object client.Object] struct {
	h           handler.TypedEventHandler[object, reconcile.Request]
	clusterName string
}

// Create implements EventHandler.
func (e *clusterHandler[object]) Create(ctx context.Context, evt event.TypedCreateEvent[object], q workqueue.TypedRateLimitingInterface[mcreconcile.Request]) {
	e.h.Create(ctx, evt, clusterQueue{q: q, cl: e.clusterName})
}

// Update implements EventHandler.
func (e *clusterHandler[object]) Update(ctx context.Context, evt event.TypedUpdateEvent[object], q workqueue.TypedRateLimitingInterface[mcreconcile.Request]) {
	e.h.Update(ctx, evt, clusterQueue{q: q, cl: e.clusterName})
}

// Delete implements EventHandler.
func (e *clusterHandler[object]) Delete(ctx context.Context, evt event.TypedDeleteEvent[object], q workqueue.TypedRateLimitingInterface[mcreconcile.Request]) {
	e.h.Delete(ctx, evt, clusterQueue{q: q, cl: e.clusterName})
}

// Generic implements EventHandler.
func (e *clusterHandler[object]) Generic(ctx context.Context, evt event.TypedGenericEvent[object], q workqueue.TypedRateLimitingInterface[mcreconcile.Request]) {
	e.h.Generic(ctx, evt, clusterQueue{q: q, cl: e.clusterName})
}

var _ workqueue.TypedRateLimitingInterface[reconcile.Request] = &clusterQueue{}

type clusterQueue struct {
	q  workqueue.TypedRateLimitingInterface[mcreconcile.Request]
	cl string
}

func (c clusterQueue) Add(item reconcile.Request) {
	c.q.Add(mcreconcile.Request{Request: item, ClusterName: c.cl})
}

func (c clusterQueue) Len() int {
	return c.q.Len()
}

func (c clusterQueue) Get() (item reconcile.Request, shutdown bool) {
	it, shutdown := c.q.Get()
	return it.Request, shutdown
}

func (c clusterQueue) Done(item reconcile.Request) {
	c.q.Done(mcreconcile.Request{Request: item, ClusterName: c.cl})
}

func (c clusterQueue) ShutDown() {
	c.q.ShutDown()
}

func (c clusterQueue) ShutDownWithDrain() {
	c.q.ShutDownWithDrain()
}

func (c clusterQueue) ShuttingDown() bool {
	return c.q.ShuttingDown()
}

func (c clusterQueue) AddAfter(item reconcile.Request, duration time.Duration) {
	c.q.AddAfter(mcreconcile.Request{Request: item, ClusterName: c.cl}, duration)
}

func (c clusterQueue) AddRateLimited(item reconcile.Request) {
	c.q.AddRateLimited(mcreconcile.Request{Request: item, ClusterName: c.cl})
}

func (c clusterQueue) Forget(item reconcile.Request) {
	c.q.Forget(mcreconcile.Request{Request: item, ClusterName: c.cl})
}

func (c clusterQueue) NumRequeues(item reconcile.Request) int {
	return c.q.NumRequeues(mcreconcile.Request{Request: item, ClusterName: c.cl})
}
