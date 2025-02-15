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

package reconcile

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Request wraps a reconcile.Request and adds the cluster name.
type Request = TypedRequest[reconcile.Request]

// TypedRequest wraps a request and adds the cluster name.
type TypedRequest[request comparable] struct {
	Request request

	ClusterName string
}

// String returns the general purpose string representation.
func (r TypedRequest[request]) String() string {
	if r.ClusterName == "" {
		return fmt.Sprintf("%s", r.Request)
	}
	return "cluster://" + r.ClusterName + string(types.Separator) + fmt.Sprintf("%s", r.Request)
}

// Cluster returns the name of the cluster that the request belongs to.
func (r TypedRequest[request]) Cluster() string {
	return r.ClusterName
}

// WithCluster sets the name of the cluster that the request belongs to.
func (r TypedRequest[request]) WithCluster(name string) TypedRequest[request] {
	r.ClusterName = name
	return r
}
