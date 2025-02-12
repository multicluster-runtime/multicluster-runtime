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
	"reflect"

	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.TypedEventHandler[client.Object, mcreconcile.Request] = &EnqueueRequestForObject{}

// EnqueueRequestForObject enqueues a reconcile.Request containing the Name, Namespace and ClusterName of the object that is the source of the Event.
// (e.g. the created / deleted / updated objects Name and Namespace). handler.EnqueueRequestForObject should be used by multi-cluster
// Controllers that have associated Resources (e.g. CRDs) to reconcile the associated Resource.
type EnqueueRequestForObject = TypedEnqueueRequestForObject[client.Object, string]

// TypedEnqueueRequestForObject enqueues a reconcile.Request containing the Name, Namespace and Cluster of the object that is the source of the Event.
// (e.g. the created / deleted / updated objects Name and Namespace).  handler.TypedEnqueueRequestForObject should be used by multi-cluster
// Controllers that have associated Resources (e.g. CRDs) to reconcile the associated Resource.
//
// TypedEnqueueRequestForObject is experimental and subject to future change.
type TypedEnqueueRequestForObject[object client.Object, cluster comparable] struct {
	Cluster cluster
}

// Create implements EventHandler.
func (e *TypedEnqueueRequestForObject[T, cluster]) Create(ctx context.Context, evt event.TypedCreateEvent[T], q workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[cluster]]) {
	if isNil(evt.Object) {
		log.FromContext(ctx).WithName("eventhandler").WithName("EnqueueRequestForObject").
			Error(nil, "CreateEvent received with no metadata", "event", evt)
		return
	}
	q.Add(mcreconcile.TypedRequest[cluster]{
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}},
		Cluster: e.Cluster,
	})
}

// Update implements EventHandler.
func (e *TypedEnqueueRequestForObject[T, cluster]) Update(ctx context.Context, evt event.TypedUpdateEvent[T], q workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[cluster]]) {
	switch {
	case !isNil(evt.ObjectNew):
		q.Add(mcreconcile.TypedRequest[cluster]{
			Request: reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      evt.ObjectNew.GetName(),
				Namespace: evt.ObjectNew.GetNamespace(),
			}},
			Cluster: e.Cluster,
		})
	case !isNil(evt.ObjectOld):
		q.Add(
			mcreconcile.TypedRequest[cluster]{
				Request: reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      evt.ObjectOld.GetName(),
					Namespace: evt.ObjectOld.GetNamespace(),
				}},
				Cluster: e.Cluster,
			})
	default:
		log.FromContext(ctx).WithName("eventhandler").WithName("EnqueueRequestForObject").
			Error(nil, "UpdateEvent received with no metadata", "event", evt)
	}
}

// Delete implements EventHandler.
func (e *TypedEnqueueRequestForObject[T, cluster]) Delete(ctx context.Context, evt event.TypedDeleteEvent[T], q workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[cluster]]) {
	if isNil(evt.Object) {
		log.FromContext(ctx).WithName("eventhandler").WithName("EnqueueRequestForObject").
			Error(nil, "DeleteEvent received with no metadata", "event", evt)
		return
	}
	q.Add(mcreconcile.TypedRequest[cluster]{
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}},
		Cluster: e.Cluster,
	})
}

// Generic implements EventHandler.
func (e *TypedEnqueueRequestForObject[T, cluster]) Generic(ctx context.Context, evt event.TypedGenericEvent[T], q workqueue.TypedRateLimitingInterface[mcreconcile.TypedRequest[cluster]]) {
	if isNil(evt.Object) {
		log.FromContext(ctx).WithName("eventhandler").WithName("EnqueueRequestForObject").
			Error(nil, "GenericEvent received with no metadata", "event", evt)
		return
	}
	q.Add(mcreconcile.TypedRequest[cluster]{
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}},
		Cluster: e.Cluster,
	})
}

func isNil(arg any) bool {
	if v := reflect.ValueOf(arg); !v.IsValid() || ((v.Kind() == reflect.Ptr ||
		v.Kind() == reflect.Interface ||
		v.Kind() == reflect.Slice ||
		v.Kind() == reflect.Map ||
		v.Kind() == reflect.Chan ||
		v.Kind() == reflect.Func) && v.IsNil()) {
		return true
	}
	return false
}
