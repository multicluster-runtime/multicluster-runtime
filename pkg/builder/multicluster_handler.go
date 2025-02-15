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

package builder

import (
	mchandler "github.com/multicluster-runtime/multicluster-runtime/pkg/handler"
	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// StaticHandler returns a handler constructor with a static cluster value.
func StaticHandler[object client.Object, request comparable](h handler.TypedEventHandler[object, mcreconcile.TypedRequest[request]]) mchandler.TypedEventHandlerFunc[object, request] {
	return func(_ string, _ cluster.Cluster) handler.TypedEventHandler[object, mcreconcile.TypedRequest[request]] {
		return h
	}
}
