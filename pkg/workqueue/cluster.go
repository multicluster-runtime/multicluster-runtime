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

package workqueue

import (
	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
)

// ClusterFair is a queue that ensures items are dequeued fairly across different
// clusters of cluster-aware requests.
type ClusterFair[request mcreconcile.ClusterAware[request]] TypedFair[request]

// NewClusterFair creates a new ClusterFair instance.
func NewClusterFair[request mcreconcile.ClusterAware[request]]() *TypedFair[request] {
	return NewTypedFair[request](func(r request) string {
		return r.Cluster()
	})
}
