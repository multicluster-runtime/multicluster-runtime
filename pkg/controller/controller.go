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

package controller

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mcmanager "github.com/multicluster-runtime/multicluster-runtime/pkg/manager"
	"github.com/multicluster-runtime/multicluster-runtime/pkg/multicluster"
	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
	mcsource "github.com/multicluster-runtime/multicluster-runtime/pkg/source"
)

// Controller implements a Kubernetes API.  A Controller manages a work queue fed reconcile.Requests
// from source.Sources.  Work is performed through the reconcile.Reconciler for each enqueued item.
// Work typically is reads and writes Kubernetes objects to make the system state match the state specified
// in the object Spec.
type Controller = TypedController[reconcile.Request]

// Options are the arguments for creating a new Controller.
type Options = controller.TypedOptions[mcreconcile.Request]

// TypedController implements an API.
type TypedController[request comparable] interface {
	controller.TypedController[mcreconcile.TypedRequest[request]]
	multicluster.Aware

	// MultiClusterWatch watches the provided Source.
	MultiClusterWatch(src mcsource.TypedSource[client.Object, request]) error
}

// New returns a new Controller registered with the Manager.  The Manager will ensure that shared Caches have
// been synced before the Controller is Started.
//
// The name must be unique as it is used to identify the controller in metrics and logs.
func New(name string, mgr mcmanager.Manager, options Options) (Controller, error) {
	return NewTyped(name, mgr, options)
}

// NewTyped returns a new typed controller registered with the Manager,
//
// The name must be unique as it is used to identify the controller in metrics and logs.
func NewTyped[request comparable](name string, mgr mcmanager.Manager, options controller.TypedOptions[mcreconcile.TypedRequest[request]]) (TypedController[request], error) {
	c, err := NewTypedUnmanaged(name, mgr, options)
	if err != nil {
		return nil, err
	}

	// Add the controller as a Manager components
	return c, mgr.Add(c)
}

// NewUnmanaged returns a new controller without adding it to the manager. The
// caller is responsible for starting the returned controller.
//
// The name must be unique as it is used to identify the controller in metrics and logs.
func NewUnmanaged(name string, mgr mcmanager.Manager, options Options) (Controller, error) {
	return NewTypedUnmanaged[reconcile.Request](name, mgr, options)
}

// NewTypedUnmanaged returns a new typed controller without adding it to the manager.
//
// The name must be unique as it is used to identify the controller in metrics and logs.
func NewTypedUnmanaged[request comparable](name string, mgr mcmanager.Manager, options controller.TypedOptions[mcreconcile.TypedRequest[request]]) (TypedController[request], error) {
	c, err := controller.NewTypedUnmanaged[mcreconcile.TypedRequest[request]](name, mgr.GetLocalManager(), options)
	if err != nil {
		return nil, err
	}
	return &mcController[request]{
		TypedController: c,
		clusters:        make(map[string]engagedCluster),
	}, nil
}

var _ TypedController[reconcile.Request] = &mcController[reconcile.Request]{}

type mcController[request comparable] struct {
	controller.TypedController[mcreconcile.TypedRequest[request]]

	lock     sync.Mutex
	clusters map[string]engagedCluster
	sources  []mcsource.TypedSource[client.Object, request]
}

type engagedCluster struct {
	name    string
	cluster cluster.Cluster
}

func (c *mcController[request]) Engage(ctx context.Context, name string, cl cluster.Cluster) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if old, ok := c.clusters[name]; ok && old.cluster == cl {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)

	// pass through in case the controller itself is cluster aware
	if ctrl, ok := c.TypedController.(multicluster.Aware); ok {
		if err := ctrl.Engage(ctx, name, cl); err != nil {
			return err
		}
	}

	// engage cluster aware instances
	for _, aware := range c.sources {
		src, err := aware.ForCluster(name, cl)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to engage for cluster %q: %w", name, err)
		}
		if err := c.TypedController.Watch(startWithinContext[request](ctx, src)); err != nil {
			cancel()
			return fmt.Errorf("failed to watch for cluster %q: %w", name, err)
		}
	}

	ec := engagedCluster{
		name:    name,
		cluster: cl,
	}
	c.clusters[name] = ec
	go func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		if c.clusters[name] == ec {
			delete(c.clusters, name)
		}
	}()

	return nil
}

func (c *mcController[request]) MultiClusterWatch(src mcsource.TypedSource[client.Object, request]) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	for name, eng := range c.clusters {
		src, err := src.ForCluster(name, eng.cluster)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to engage for cluster %q: %w", name, err)
		}
		if err := c.TypedController.Watch(startWithinContext[request](ctx, src)); err != nil {
			cancel()
			return fmt.Errorf("failed to watch for cluster %q: %w", name, err)
		}
	}

	c.sources = append(c.sources, src)

	return nil
}

func startWithinContext[request comparable](ctx context.Context, src source.TypedSource[mcreconcile.TypedRequest[request]]) source.TypedSource[mcreconcile.TypedRequest[request]] {
	return source.TypedFunc[mcreconcile.TypedRequest[request]](func(ctlCtx context.Context, w workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[request]]) error {
		ctx, cancel := context.WithCancel(ctx)
		go func() {
			<-ctlCtx.Done()
			cancel()
		}()
		return src.Start(ctx, w)
	})
}
