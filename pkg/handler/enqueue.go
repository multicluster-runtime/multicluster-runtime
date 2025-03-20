/*
Copyright 2018 The Kubernetes Authors.

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

// EnqueueRequestForObject wraps a controller-runtime handler.EnqueueRequestForOwner
// to be compatible with multi-cluster.
func EnqueueRequestForObject(clusterName string, cl cluster.Cluster) handler.TypedEventHandler[client.Object, mcreconcile.Request] {
	return Lift(&handler.EnqueueRequestForObject{})(clusterName, cl)
}

// TypedEnqueueRequestForObject wraps a controller-runtime handler.TypedEnqueueRequestForObject
// to be compatible with multi-cluster.
func TypedEnqueueRequestForObject[object client.Object]() TypedEventHandlerFunc[object, mcreconcile.Request] {
	return TypedLift[object](&handler.TypedEnqueueRequestForObject[object]{})
}

// WithLowPriorityWhenUnchanged wraps a controller-runtime handler.WithLowPriorityWhenUnchanged
// to be compatible with multi-cluster.
func WithLowPriorityWhenUnchanged[object client.Object, request mcreconcile.ClusterAware[request]](u TypedEventHandlerFunc[object, request]) TypedEventHandlerFunc[object, request] {
	return func(clusterName string, cl cluster.Cluster) handler.TypedEventHandler[object, request] {
		return handler.WithLowPriorityWhenUnchanged[object, request](u(clusterName, cl))
	}
}
