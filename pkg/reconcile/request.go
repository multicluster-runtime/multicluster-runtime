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

// ClusterAware is a request that is aware of the cluster it belongs to.
type ClusterAware[request comparable] interface {
	comparable
	fmt.Stringer

	Cluster() string
	WithCluster(string) request
}

// Request extends a reconcile.Request by adding the cluster name.
type Request struct {
	reconcile.Request

	// ClusterName is the name of the cluster that the request belongs to.
	ClusterName string
}

// String returns the general purpose string representation.
func (r Request) String() string {
	if r.ClusterName == "" {
		return r.Request.String()
	}
	return "cluster://" + r.ClusterName + string(types.Separator) + r.Request.String()
}

// Cluster returns the name of the cluster that the request belongs to.
func (r Request) Cluster() string {
	return r.ClusterName
}

// WithCluster sets the name of the cluster that the request belongs to.
func (r Request) WithCluster(name string) Request {
	r.ClusterName = name
	return r
}
