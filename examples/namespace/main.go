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
	"errors"
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcbuilder "github.com/multicluster-runtime/multicluster-runtime/pkg/builder"
	mcmanager "github.com/multicluster-runtime/multicluster-runtime/pkg/manager"
	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"
	"github.com/multicluster-runtime/multicluster-runtime/providers/namespace"
)

func init() {
	ctrl.SetLogger(klog.Background())
}

func main() {
	log.SetLogger(zap.New(zap.UseDevMode(true)))
	entryLog := log.Log.WithName("entrypoint")
	ctx := signals.SetupSignalHandler()

	kubeconfig := flag.String("kubeconfig", "", "path to the kubeconfig file. If not given a test env is started.")
	flag.Parse()

	var cfg *rest.Config
	if *kubeconfig == "" {
		testEnv := &envtest.Environment{}
		var err error
		cfg, err = testEnv.Start()
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
	} else {
		var err error
		cfg, err = ctrl.GetConfig()
		if err != nil {
			entryLog.Error(err, "failed to get kubeconfig")
			os.Exit(1)
		}
	}

	// Test fixtures
	cli, err := client.New(cfg, client.Options{})
	if err != nil {
		entryLog.Error(err, "failed to create client")
		os.Exit(1)
	}

	entryLog.Info("Creating Namespace and ConfigMap objects")
	runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "zoo"}})))
	runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "zoo", Name: "elephant"}})))
	runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "zoo", Name: "lion"}})))
	runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "jungle"}})))
	runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "jungle", Name: "monkey"}})))
	runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "island"}})))
	runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "island", Name: "bird"}})))

	entryLog.Info("Setting up provider")
	cl, err := cluster.New(cfg, namespace.WithClusterNameIndex())
	if err != nil {
		entryLog.Error(err, "unable to set up provider")
		os.Exit(1)
	}
	provider := namespace.NewNamespacedClusterProvider(cl)

	// Setup a cluster-aware Manager, with the provider to lookup clusters.
	entryLog.Info("Setting up cluster-aware manager")
	mgr, err := mcmanager.New(cfg, provider, manager.Options{
		NewCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
			// wrap cache to turn IndexField calls into cluster-scoped indexes.
			return &namespace.NamespaceScopeableCache{Cache: cl.GetCache()}, nil
		},
	})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	if err := mcbuilder.ControllerManagedBy(mgr).
		Named("fleet-ns-configmap-controller").
		For(&corev1.ConfigMap{}).
		Complete(mcreconcile.Func(
			func(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
				log := log.FromContext(ctx).WithValues("cluster", req.Cluster)

				cl, err := mgr.GetCluster(ctx, req.Cluster)
				if err != nil {
					return reconcile.Result{}, err
				}
				client := cl.GetClient()

				// Retrieve the service account from the namespace.
				cm := &corev1.ConfigMap{}
				if err := client.Get(ctx, req.NamespacedName, cm); err != nil {
					return reconcile.Result{}, err
				}
				log.Info("Reconciling configmap", "cluster", req.Cluster, "ns", req.Namespace, "name", cm.Name, "uuid", cm.UID)

				return ctrl.Result{}, nil
			},
		)); err != nil {
		entryLog.Error(err, "unable to set up controller")
		os.Exit(1)
	}

	entryLog.Info("Starting provider")
	go func() {
		if err := ignoreCanceled(provider.Start(ctx, mgr)); err != nil {
			entryLog.Error(err, "failed to start provider")
			os.Exit(1)
		}
	}()

	entryLog.Info("Starting cluster")
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := ignoreCanceled(cl.Start(ctx)); err != nil {
			return fmt.Errorf("failed to start cluster backing provider: %w", err)
		}
		return nil
	})

	entryLog.Info("Starting cluster-aware manager")
	if err := ignoreCanceled(mgr.Start(ctx)); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}

func ignoreCanceled(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
