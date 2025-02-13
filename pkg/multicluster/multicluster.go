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

package multicluster

import (
	"context"

	ctrlcluster "sigs.k8s.io/controller-runtime/pkg/cluster"
)

// Aware is an interface that can be implemented by components that
// can engage and disengage when clusters are added or removed at runtime.
type Aware[cluster comparable] interface {
	// Engage gets called when the component should start operations for the given Cluster.
	// The given context is tied to the Cluster's lifecycle and will be cancelled when the
	// Cluster is removed or an error occurs.
	//
	// Implementers should return an error if they cannot start operations for the given Cluster,
	// and should ensure this operation is re-entrant and non-blocking.
	//
	//	\_________________|)____.---'--`---.____
	//              ||    \----.________.----/
	//              ||     / /    `--'
	//            __||____/ /_
	//           |___         \
	//               `--------'
	Engage(context.Context, cluster, ctrlcluster.Cluster) error
}

// Provider is the TypedProvider with string as the cluster type.
type Provider = TypedProvider[string]

// TypedProvider allows to retrieve clusters by name. The provider is responsible for discovering
// and managing the lifecycle of each cluster, calling `Engage` on the manager
// it is run against whenever a new cluster is discovered and cancelling the
// context used on engage when a cluster is unregistered.
//
// Example: A Cluster API provider would be responsible for discovering and
// managing clusters that are backed by Cluster API resources, which can live
// in multiple namespaces in a single management cluster.
type TypedProvider[cluster comparable] interface {
	// Get returns a cluster for the given identifying cluster name. Get
	// returns an existing cluster if it has been created before.
	// If no cluster is known to the provider under the given cluster name,
	// an error should be returned.
	Get(ctx context.Context, cl cluster) (ctrlcluster.Cluster, error)
}
