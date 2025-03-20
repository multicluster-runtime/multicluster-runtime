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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

// EnqueueRequestForOwner wraps handler.EnqueueRequestForOwner to be compatible
// with multi-cluster.
func EnqueueRequestForOwner(ownerType client.Object, opts ...handler.OwnerOption) EventHandlerFunc {
	return func(clusterName string, cl cluster.Cluster) EventHandler {
		return Lift(handler.EnqueueRequestForOwner(cl.GetScheme(), cl.GetRESTMapper(), ownerType, opts...))(clusterName, cl)
	}
}

// TypedEnqueueRequestForOwner wraps handler.TypedEnqueueRequestForOwner to be
// compatible with multi-cluster.
func TypedEnqueueRequestForOwner[object client.Object](ownerType client.Object, opts ...handler.OwnerOption) TypedEventHandlerFunc[object, mcreconcile.Request] {
	return func(clusterName string, cl cluster.Cluster) handler.TypedEventHandler[object, mcreconcile.Request] {
		return TypedLift[object](handler.TypedEnqueueRequestForOwner[object](cl.GetScheme(), cl.GetRESTMapper(), ownerType, opts...))(clusterName, cl)
	}
}
