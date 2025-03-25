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

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	kubeconfig "sigs.k8s.io/multicluster-runtime/providers/kubeconfig"

	// +kubebuilder:scaffold:imports
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = zap.New(zap.UseDevMode(true))
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// TODO: add your CRDs here after you import them above
	// utilruntime.Must(crdv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var masterURL string
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Create config with explicitly provided kubeconfig/master
	// Note: controller-runtime already handles the --kubeconfig flag
	var config *rest.Config
	var err error

	// Get kubeconfig path from the flag that controller-runtime registers
	kubeconfigPath := os.Getenv("KUBECONFIG")
	for i := 1; i < len(os.Args); i++ {
		if strings.HasPrefix(os.Args[i], "--kubeconfig=") {
			kubeconfigPath = strings.TrimPrefix(os.Args[i], "--kubeconfig=")
			break
		}
		if os.Args[i] == "--kubeconfig" && i+1 < len(os.Args) {
			kubeconfigPath = os.Args[i+1]
			break
		}
	}

	if masterURL != "" {
		setupLog.Info("Using explicitly provided master URL", "master", masterURL)
		// Use explicit master URL with kubeconfig if provided
		if kubeconfigPath != "" {
			setupLog.Info("Using kubeconfig file with explicit master", "kubeconfig", kubeconfigPath)
			config, err = clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
		} else {
			// Just use the master URL
			config = &rest.Config{
				Host: masterURL,
			}
		}
	} else {
		// Use controller-runtime's standard config handling
		setupLog.Info("Using controller-runtime config handling")
		if kubeconfigPath != "" {
			setupLog.Info("Using kubeconfig file", "path", kubeconfigPath)
		}
		config, err = ctrl.GetConfig()
	}

	if err != nil {
		setupLog.Error(err, "unable to get kubernetes configuration")
		os.Exit(1)
	}

	setupLog.Info("Successfully connected to Kubernetes API", "host", config.Host)

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// Metrics endpoint options
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// Get the root context for the whole application
	ctx := ctrl.SetupSignalHandler()

	// Create a context with cancel for graceful shutdown of background tasks
	ctxWithCancel, cancelFunc := context.WithCancel(ctx)

	// Make sure to cancel all background processes when main context is done
	go func() {
		<-ctx.Done()
		setupLog.Info("Main context cancelled, shutting down background tasks")
		cancelFunc()
	}()

	// Determine the namespace for kubeconfig discovery
	namespace, err := getOperatorNamespace()
	if err != nil {
		setupLog.Error(err, "unable to determine operator namespace")
		os.Exit(1)
	}

	setupLog.Info("initializing multicluster support", "namespace", namespace)

	// Create standard manager options
	mgmtOpts := manager.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "cb9167b4.hahomelabs.com",
	}

	// First, create a standard controller-runtime manager
	// This is for the conventional controller approach
	mgr, err := ctrl.NewManager(config, mgmtOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Now let's set up the multicluster part
	// Create a MulticlusterReconciler that will be used by controllers and the provider
	mcReconciler := kubeconfig.NewMulticlusterReconciler(mgr.GetClient(), mgr.GetScheme())

	// Create a KubeconfigClusterManager that will manage multiple clusters
	clusterManager := kubeconfig.NewKubeconfigClusterManager(mgr, mcReconciler)

	// Create and configure the kubeconfig provider with the cluster manager
	kubeconfigProvider := kubeconfig.New(
		clusterManager,
		kubeconfig.Options{
			Namespace:         namespace,
			KubeconfigLabel:   "sigs.k8s.io/multicluster-runtime-kubeconfig", // TODO: change this to your desired kubeconfig label
			Scheme:            scheme,
			ConnectionTimeout: 15 * time.Second, // TODO: change this to your operator's connection timeout
			CacheSyncTimeout:  60 * time.Second, // TODO: change this to your operator's cache sync timeout
		},
	)

	// Start the provider in a background goroutine and wait for initial discovery
	providerReady := make(chan struct{})
	go func() {
		setupLog.Info("starting kubeconfig provider")

		// Set the manager first before doing anything else
		kubeconfigProvider.SetManager(clusterManager)

		// Signal that we're going to start the provider
		// This doesn't mean clusters are actually ready yet, just that we're starting
		close(providerReady)

		// Run the provider - it will handle initial sync internally
		err := kubeconfigProvider.Run(ctxWithCancel, clusterManager)
		if err != nil && !errors.Is(err, context.Canceled) {
			setupLog.Error(err, "Error running provider")
			os.Exit(1)
		}
	}()

	// Wait for the provider to start
	select {
	case <-providerReady:
		setupLog.Info("Kubeconfig provider starting...")
	case <-time.After(5 * time.Second):
		setupLog.Info("Timeout waiting for provider to start, continuing anyway")
	}

	// Now let's add a delay to allow provider to discover and connect to clusters
	setupLog.Info("Waiting for provider to discover clusters...")
	time.Sleep(5 * time.Second)

	// TODO: set up your controllers for CRDs/CRs. 
	//   Below are examples for 'FailoverGroup' and 'Failover' CRs.

	// if err := (&controller.FailoverGroupReconciler{
	// 	Client:       mgr.GetClient(),
	// 	Scheme:       mgr.GetScheme(),
	// 	MCReconciler: mcReconciler,
	// }).SetupWithManager(mgr); err != nil {
	// 	setupLog.Error(err, "unable to create failovergroup controller", "controller", "FailoverGroup")
	// 	os.Exit(1)
	// }

	// if err := (&controller.FailoverReconciler{
	// 	Client:       mgr.GetClient(),
	// 	Scheme:       mgr.GetScheme(),
	// 	MCReconciler: mcReconciler,
	// }).SetupWithManager(mgr); err != nil {
	// 	setupLog.Error(err, "unable to create failover controller", "controller", "Failover")
	// 	os.Exit(1)
	// }

	// Setup healthz/readyz checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// getOperatorNamespace returns the namespace the operator is currently running in.
func getOperatorNamespace() (string, error) {
	// Check if running in a pod
	ns, found := os.LookupEnv("POD_NAMESPACE")
	if found {
		return ns, nil
	}

	// If not running in a pod, try to get from the service account namespace
	nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil {
		return string(nsBytes), nil
	}

	// Default to the standard operator namespace
	return "my-operator-namespace", nil // TODO: change this to your operator's namespace
}
