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

package namespace

import (
	"context"
	"errors"

	"golang.org/x/sync/errgroup"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcbuilder "github.com/multicluster-runtime/multicluster-runtime/pkg/builder"
	mcmanager "github.com/multicluster-runtime/multicluster-runtime/pkg/manager"
	mcreconcile "github.com/multicluster-runtime/multicluster-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Provider Namespace", func() {
	Describe("New", func() {
		It("should return success if given valid objects", func(ctx context.Context) {
			cli, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			By("Creating Namespace and ConfigMap objects")
			runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "zoo"}})))
			runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "zoo", Name: "elephant", Labels: map[string]string{"type": "animal"}}})))
			runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "zoo", Name: "lion", Labels: map[string]string{"type": "animal"}}})))
			runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "jungle"}})))
			runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "jungle", Name: "monkey", Labels: map[string]string{"type": "animal"}}})))
			runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "island"}})))
			runtime.Must(client.IgnoreAlreadyExists(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "island", Name: "bird", Labels: map[string]string{"type": "animal"}}})))

			By("Setting up the provider")
			cl, err := cluster.New(cfg, WithClusterNameIndex(), func(options *cluster.Options) {
				options.Cache.ByObject = map[client.Object]cache.ByObject{
					&corev1.ConfigMap{}: {
						Label: labels.Set{"type": "animal"}.AsSelector(),
					},
				}
			})
			Expect(err).NotTo(HaveOccurred())
			provider := New(cl)

			By("Setting up the cluster-aware manager, with the provider to lookup clusters")
			mgr, err := mcmanager.New(cfg, provider, manager.Options{
				NewCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
					// wrap cache to turn IndexField calls into cluster-scoped indexes.
					return &NamespaceScopeableCache{Cache: cl.GetCache()}, nil
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Setting up the controller")
			err = mcbuilder.ControllerManagedBy(mgr).
				Named("fleet-ns-configmap-controller").
				For(&corev1.ConfigMap{}).
				Complete(mcreconcile.Func(
					func(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
						log := log.FromContext(ctx).WithValues("request", req.String())
						log.Info("Reconciling ConfigMap")

						cl, err := mgr.GetCluster(ctx, req.ClusterName)
						if err != nil {
							return reconcile.Result{}, err
						}

						// Feed the animal.
						cm := &corev1.ConfigMap{}
						if err := cl.GetClient().Get(ctx, req.NamespacedName, cm); err != nil {
							if apierrors.IsNotFound(err) {
								return reconcile.Result{}, nil
							}
							return reconcile.Result{}, err
						}
						cm.Data = map[string]string{"stomach": "food"}
						if err := cl.GetClient().Update(ctx, cm); err != nil {
							return reconcile.Result{}, err
						}

						return ctrl.Result{}, nil
					},
				))
			Expect(err).NotTo(HaveOccurred())

			By("Starting provider")
			ctx, cancel := context.WithCancel(ctx)
			g, ctx := errgroup.WithContext(ctx)
			defer func() {
				cancel()
				By("Waiting for all components to finish")
				err = g.Wait()
				Expect(err).NotTo(HaveOccurred())
			}()
			g.Go(func() error {
				return ignoreCanceled(provider.Run(ctx, mgr))
			})

			By("Starting cluster")
			g.Go(func() error {
				return ignoreCanceled(cl.Start(ctx))
			})

			By("Starting cluster-aware manager")
			g.Go(func() error {
				return ignoreCanceled(mgr.Start(ctx))
			})

			err = cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "zoo", Name: "tiger", Labels: map[string]string{"type": "animal"}}})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				cm := &corev1.ConfigMap{}
				err := cli.Get(ctx, client.ObjectKey{Namespace: "zoo", Name: "tiger"}, cm)
				Expect(err).NotTo(HaveOccurred())
				return cm.Data["stomach"]
			}, "10s").Should(Equal("food"))
		})
	})
})

func ignoreCanceled(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
