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

	kerrors "k8s.io/apimachinery/pkg/util/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	mcmanager "github.com/multicluster-runtime/multicluster-runtime/pkg/manager"
	"github.com/multicluster-runtime/multicluster-runtime/pkg/multicluster"
	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
	mcsource "github.com/multicluster-runtime/multicluster-runtime/pkg/source"
)

// Controller implements a Kubernetes API.  A Controller manages a work queue fed reconcile.Requests
// from source.Sources.  Work is performed through the reconcile.Reconciler for each enqueued item.
// Work typically is reads and writes Kubernetes objects to make the system state match the state specified
// in the object Spec.
type Controller = TypedController[mcreconcile.Request]

// Options are the arguments for creating a new Controller.
type Options = controller.TypedOptions[mcreconcile.Request]

// TypedController implements an API.
type TypedController[request comparable] interface {
	controller.TypedController[request]
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
func NewTyped[request comparable](name string, mgr mcmanager.Manager, options controller.TypedOptions[request]) (TypedController[request], error) {
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
	return NewTypedUnmanaged[mcreconcile.Request](name, mgr, options)
}

// NewTypedUnmanaged returns a new typed controller without adding it to the manager.
//
// The name must be unique as it is used to identify the controller in metrics and logs.
func NewTypedUnmanaged[request comparable](name string, mgr mcmanager.Manager, options controller.TypedOptions[request]) (TypedController[request], error) {
	c, err := controller.NewTypedUnmanaged[request](name, mgr.GetHostManager(), options)
	if err != nil {
		return nil, err
	}
	return &mcController[request]{
		TypedController: c,
		clusters:        make(map[string]engagedCluster),
	}, nil
}

var _ TypedController[mcreconcile.Request] = &mcController[mcreconcile.Request]{}

type mcController[request comparable] struct {
	controller.TypedController[request]

	lock     sync.Mutex
	clusters map[string]engagedCluster
	sources  []mcsource.TypedSource[client.Object, request]
}

type engagedCluster struct {
	name    string
	cluster cluster.Cluster
	cancel  context.CancelFunc
}

func (c *mcController[request]) Engage(ctx context.Context, name string, cl cluster.Cluster) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.clusters[name]; ok {
		return nil
	}

	cancels := make([]func() error, 0, len(c.sources)+1)
	disengage := func(err error) error {
		var errs []error
		for _, cancel := range cancels {
			if err := cancel(); err != nil {
				errs = append(errs, err)
			}
		}
		return kerrors.NewAggregate(errs)
	}

	// pass through in case the controller itself is cluster aware
	if ctrl, ok := c.TypedController.(multicluster.Aware); ok {
		if err := ctrl.Engage(ctx, name, cl); err != nil {
			return err
		}
		cancels = append(cancels, func() error {
			return ctrl.Disengage(ctx, name)
		})
	}

	// engage cluster aware instances
	for _, aware := range c.sources {
		src, cancel, err := aware.Engage(cl)
		if err != nil {
			return disengage(fmt.Errorf("failed to engage for cluster %q: %w", name, err))
		}
		if err := c.TypedController.Watch(src); err != nil {
			return disengage(fmt.Errorf("failed to watch for cluster %q: %w", name, err))
		}
		cancels = append(cancels, func() error {
			cancel()
			return nil
		})
	}

	c.clusters[name] = engagedCluster{
		name:    name,
		cluster: cl,
	}

	return nil
}

func (c *mcController[request]) Disengage(ctx context.Context, name string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.clusters[name]; !ok {
		return nil
	}
	delete(c.clusters, name)

	// pass through in case the controller itself is cluster aware
	var errs []error
	if ctrl, ok := c.TypedController.(multicluster.Aware); ok {
		if err := ctrl.Disengage(ctx, name); err != nil {
			errs = append(errs, err)
		}
	}

	return kerrors.NewAggregate(errs)
}

func (c *mcController[request]) MultiClusterWatch(src mcsource.TypedSource[client.Object, request]) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.sources = append(c.sources, src)

	cancels := make([]func() error, 0, len(c.sources)+1)
	disengage := func(err error) error {
		var errs []error
		for _, cancel := range cancels {
			if err := cancel(); err != nil {
				errs = append(errs, err)
			}
		}
		return kerrors.NewAggregate(errs)
	}

	for name, eng := range c.clusters {
		src, cancel, err := src.Engage(eng.cluster)
		if err != nil {
			return disengage(fmt.Errorf("failed to engage for cluster %q: %w", name, err))
		}
		if err := c.TypedController.Watch(src); err != nil {
			return disengage(fmt.Errorf("failed to watch for cluster %q: %w", name, err))
		}
		cancels = append(cancels, func() error {
			cancel()
			return nil
		})
	}

	return nil
}
