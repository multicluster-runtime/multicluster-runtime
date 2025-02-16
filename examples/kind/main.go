/*
Copyright 2024 The Kubernetes Authors.

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

package main

import (
	"context"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcbuilder "github.com/multicluster-runtime/multicluster-runtime/pkg/builder"
	mcmanager "github.com/multicluster-runtime/multicluster-runtime/pkg/manager"
	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
	"github.com/multicluster-runtime/multicluster-runtime/providers/kind"
)

func main() {
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	ctx := signals.SetupSignalHandler()
	entryLog := log.Log.WithName("entrypoint")

	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil {
		entryLog.Error(err, "failed to start local environment")
		os.Exit(1)
	}
	defer func() {
		if testEnv == nil {
			return
		}
		if err := testEnv.Stop(); err != nil {
			entryLog.Error(err, "failed to stop local environment")
			os.Exit(1)
		}
	}()

	// Setup a Manager, note that this not yet engages clusters, only makes them available.
	entryLog.Info("Setting up manager")
	provider := kind.New()
	mgr, err := mcmanager.New(cfg, provider, manager.Options{})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1) //nolint:gocritic // We want to return an error RC.
	}

	if err := mcbuilder.ControllerManagedBy(mgr).
		Named("fleet-pod-controller").
		For(&corev1.Pod{}).
		Complete(mcreconcile.Func(
			func(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
				log := log.FromContext(ctx).WithValues("cluster", req.ClusterName)

				cl, err := mgr.GetCluster(ctx, req.ClusterName)
				if err != nil {
					return reconcile.Result{}, err
				}
				client := cl.GetClient()

				// Retrieve the pod from the cluster.
				pod := &corev1.Pod{}
				if err := client.Get(ctx, req.Request.NamespacedName, pod); err != nil {
					if !apierrors.IsNotFound(err) {
						return reconcile.Result{}, err
					}
					// Pod was deleted.
					return reconcile.Result{}, nil
				}

				// If the pod is being deleted, we can skip it.
				if pod.DeletionTimestamp != nil {
					return reconcile.Result{}, nil
				}

				log.Info("Reconciling pod", "ns", pod.GetNamespace(), "name", pod.Name, "uuid", pod.UID)

				// Print any annotations that start with fleet.
				for k, v := range pod.Labels {
					if strings.HasPrefix(k, "fleet-") {
						log.Info("Detected fleet label!", "pod", pod.Name, "key", k, "value", v)
					}
				}

				return ctrl.Result{}, nil
			},
		)); err != nil {
		entryLog.Error(err, "failed to build controller")
		os.Exit(1)
	}

	entryLog.Info("Starting provider")
	go func() {
		if err := provider.Run(ctx, mgr); err != nil {
			entryLog.Error(err, "unable to run provider")
			os.Exit(1)
		}
	}()

	entryLog.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}
