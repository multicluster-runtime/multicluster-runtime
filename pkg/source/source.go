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

package source

import (
	"context"

	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
)

// Source is a source of events (e.g. Create, Update, Delete operations on Kubernetes Objects, Webhook callbacks, etc)
// which should be processed by event.EventHandlers to enqueue reconcile.Requests.
//
// * Use Kind for events originating in the cluster (e.g. Pod Create, Pod Update, Deployment Update).
//
// * Use Channel for events originating outside the cluster (e.g. GitHub Webhook callback, Polling external urls).
//
// Users may build their own Source implementations.
type Source = TypedSource[client.Object, mcreconcile.Request]

// CancelFunc is a function that can be called to cancel the source.
type CancelFunc = func()

// TypedSource is a generic source of events (e.g. Create, Update, Delete operations on Kubernetes Objects, Webhook callbacks, etc)
// which should be processed by event.EventHandlers to enqueue a request.
//
// * Use Kind for events originating in the cluster (e.g. Pod Create, Pod Update, Deployment Update).
//
// * Use Channel for events originating outside the cluster (e.g. GitHub Webhook callback, Polling external urls).
//
// Users may build their own Source implementations.
type TypedSource[object client.Object, request comparable] interface {
	Engage(cluster.Cluster) (source.TypedSource[request], CancelFunc, error)
}

// SyncingSource is a source that needs syncing prior to being usable. The controller
// will call its WaitForSync prior to starting workers.
type SyncingSource[object client.Object] TypedSyncingSource[object, mcreconcile.Request]

// TypedSyncingSource is a source that needs syncing prior to being usable. The controller
// will call its WaitForSync prior to starting workers.
type TypedSyncingSource[object client.Object, request comparable] interface {
	TypedSource[object, request]
	SyncingEngage(cluster.Cluster) (source.TypedSyncingSource[request], CancelFunc, error)
	WithProjection(func(cluster.Cluster, object) (object, error)) TypedSyncingSource[object, request]
}

// Kind creates a KindSource with the given cache provider.
func Kind[object client.Object](
	obj object,
	handler func(cluster.Cluster) handler.TypedEventHandler[object, mcreconcile.Request],
	predicates ...predicate.TypedPredicate[object],
) SyncingSource[object] {
	return TypedKind[object, mcreconcile.Request](obj, handler, predicates...)
}

// TypedKind creates a KindSource with the given cache provider.
func TypedKind[object client.Object, request comparable](
	obj object,
	handler func(cluster.Cluster) handler.TypedEventHandler[object, request],
	predicates ...predicate.TypedPredicate[object],
) TypedSyncingSource[object, request] {
	return &kind[object, request]{
		obj:        obj,
		handler:    handler,
		predicates: predicates,
		project:    func(_ cluster.Cluster, obj object) (object, error) { return obj, nil },
	}
}

type kind[object client.Object, request comparable] struct {
	obj        object
	handler    func(cl cluster.Cluster) handler.TypedEventHandler[object, request]
	predicates []predicate.TypedPredicate[object]
	project    func(cluster.Cluster, object) (object, error)
}

type engagedKind[object client.Object, request comparable] struct {
	source.TypedSyncingSource[request]
	done chan struct{}
}

// WithProjection sets the projection function for the KindSource.
func (k *kind[object, request]) WithProjection(project func(cluster.Cluster, object) (object, error)) TypedSyncingSource[object, request] {
	k.project = project
	return k
}

func (k *kind[object, request]) Engage(cl cluster.Cluster) (source.TypedSource[request], CancelFunc, error) {
	obj, err := k.project(cl, k.obj)
	if err != nil {
		return nil, nil, err
	}
	stopCh := make(chan struct{})
	return &engagedKind[object, request]{
		TypedSyncingSource: source.TypedKind(cl.GetCache(), obj, k.handler(cl), k.predicates...),
		done:               stopCh,
	}, func() { close(stopCh) }, nil
}

func (k *kind[object, request]) SyncingEngage(cl cluster.Cluster) (source.TypedSyncingSource[request], CancelFunc, error) {
	obj, err := k.project(cl, k.obj)
	if err != nil {
		return nil, nil, err
	}
	stopCh := make(chan struct{})
	return &engagedKind[object, request]{
		TypedSyncingSource: source.TypedKind(cl.GetCache(), obj, k.handler(cl), k.predicates...),
		done:               stopCh,
	}, func() { close(stopCh) }, nil
}

func (k *engagedKind[object, request]) Start(ctx context.Context, q workqueue.TypedRateLimitingInterface[request]) error {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return k.TypedSyncingSource.Start(ctx, q)
}

func (k *engagedKind[object, request]) WaitForSync(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return k.TypedSyncingSource.WaitForSync(ctx)
}
