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
type EnqueueRequestForObject = TypedEnqueueRequestForObject[client.Object]

// TypedEnqueueRequestForObject enqueues a reconcile.Request containing the Name, Namespace and ClusterName of the object that is the source of the Event.
// (e.g. the created / deleted / updated objects Name and Namespace).  handler.TypedEnqueueRequestForObject should be used by multi-cluster
// Controllers that have associated Resources (e.g. CRDs) to reconcile the associated Resource.
//
// TypedEnqueueRequestForObject is experimental and subject to future change.
type TypedEnqueueRequestForObject[object client.Object] struct {
	ClusterName string
}

// Create implements EventHandler.
func (e *TypedEnqueueRequestForObject[T]) Create(ctx context.Context, evt event.TypedCreateEvent[T], q workqueue.TypedRateLimitingInterface[mcreconcile.Request]) {
	if isNil(evt.Object) {
		log.FromContext(ctx).WithName("eventhandler").WithName("EnqueueRequestForObject").
			Error(nil, "CreateEvent received with no metadata", "event", evt)
		return
	}
	q.Add(mcreconcile.Request{
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}},
		ClusterName: e.ClusterName,
	})
}

// Update implements EventHandler.
func (e *TypedEnqueueRequestForObject[T]) Update(ctx context.Context, evt event.TypedUpdateEvent[T], q workqueue.TypedRateLimitingInterface[mcreconcile.Request]) {
	switch {
	case !isNil(evt.ObjectNew):
		q.Add(mcreconcile.Request{
			Request: reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      evt.ObjectNew.GetName(),
				Namespace: evt.ObjectNew.GetNamespace(),
			}},
			ClusterName: e.ClusterName,
		})
	case !isNil(evt.ObjectOld):
		q.Add(
			mcreconcile.Request{
				Request: reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      evt.ObjectOld.GetName(),
					Namespace: evt.ObjectOld.GetNamespace(),
				}},
				ClusterName: e.ClusterName,
			})
	default:
		log.FromContext(ctx).WithName("eventhandler").WithName("EnqueueRequestForObject").
			Error(nil, "UpdateEvent received with no metadata", "event", evt)
	}
}

// Delete implements EventHandler.
func (e *TypedEnqueueRequestForObject[T]) Delete(ctx context.Context, evt event.TypedDeleteEvent[T], q workqueue.TypedRateLimitingInterface[mcreconcile.Request]) {
	if isNil(evt.Object) {
		log.FromContext(ctx).WithName("eventhandler").WithName("EnqueueRequestForObject").
			Error(nil, "DeleteEvent received with no metadata", "event", evt)
		return
	}
	q.Add(mcreconcile.Request{
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}},
		ClusterName: e.ClusterName,
	})
}

// Generic implements EventHandler.
func (e *TypedEnqueueRequestForObject[T]) Generic(ctx context.Context, evt event.TypedGenericEvent[T], q workqueue.TypedRateLimitingInterface[mcreconcile.Request]) {
	if isNil(evt.Object) {
		log.FromContext(ctx).WithName("eventhandler").WithName("EnqueueRequestForObject").
			Error(nil, "GenericEvent received with no metadata", "event", evt)
		return
	}
	q.Add(mcreconcile.Request{
		Request: reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      evt.Object.GetName(),
			Namespace: evt.Object.GetNamespace(),
		}},
		ClusterName: e.ClusterName,
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
