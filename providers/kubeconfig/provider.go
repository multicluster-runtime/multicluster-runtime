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

// Package kubeconfig provides a Kubernetes cluster provider that watches secrets
// containing kubeconfig data and creates controller-runtime clusters for each.
package kubeconfig

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// DefaultKubeconfigSecretLabel is the default label key to identify kubeconfig secrets
	DefaultKubeconfigSecretLabel = "sigs.k8s.io/multicluster-runtime-kubeconfig"

	// DefaultKubeconfigSecretKey is the default key in the secret data that contains the kubeconfig
	DefaultKubeconfigSecretKey = "kubeconfig"
)

// index defines a field indexer
type index struct {
	object       client.Object
	field        string
	extractValue client.IndexerFunc
}

// Options are the options for the Kubeconfig Provider.
type Options struct {
	// Namespace to watch for kubeconfig secrets
	Namespace string

	// Label key to identify kubeconfig secrets
	KubeconfigLabel string

	// Key in the secret data that contains the kubeconfig
	KubeconfigKey string

	// Scheme is the scheme to use for the cluster. If not provided, a new one will be created.
	Scheme *runtime.Scheme

	// ConnectionTimeout is the timeout for connecting to a cluster
	ConnectionTimeout time.Duration

	// CacheSyncTimeout is the timeout for waiting for the cache to sync
	CacheSyncTimeout time.Duration
}

// KubeconfigProvider is a cluster provider that watches for secrets containing kubeconfig data
// and engages clusters based on those kubeconfig.
type KubeconfigProvider struct {
	opts       Options
	log        logr.Logger
	client     client.Client
	Client     client.Client // For controller-runtime Reconciler interface
	lock       sync.RWMutex
	manager    KubeClusterManager
	clusters   map[string]cluster.Cluster
	cancelFns  map[string]context.CancelFunc
	indexers   []index
	seenHashes map[string]string // tracks resource versions
}

// Ensure KubeconfigProvider implements the Provider interface
var _ Provider = &KubeconfigProvider{}

// New creates a new Kubeconfig Provider.
func New(mgr KubeClusterManager, opts Options) *KubeconfigProvider {
	// Set defaults
	if opts.KubeconfigLabel == "" {
		opts.KubeconfigLabel = DefaultKubeconfigSecretLabel
	}
	if opts.KubeconfigKey == "" {
		opts.KubeconfigKey = DefaultKubeconfigSecretKey
	}
	if opts.ConnectionTimeout == 0 {
		opts.ConnectionTimeout = 10 * time.Second
	}
	if opts.CacheSyncTimeout == 0 {
		opts.CacheSyncTimeout = 30 * time.Second
	}

	return &KubeconfigProvider{
		opts:       opts,
		log:        log.Log.WithName("kubeconfig-provider"),
		client:     mgr.GetClient(),
		Client:     mgr.GetClient(), // Set both client fields
		clusters:   map[string]cluster.Cluster{},
		cancelFns:  map[string]context.CancelFunc{},
		seenHashes: map[string]string{},
	}
}

// Get returns the cluster with the given name, if it is known.
// It implements the Provider interface.
func (p *KubeconfigProvider) Get(_ context.Context, clusterName string) (cluster.Cluster, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if cl, ok := p.clusters[clusterName]; ok {
		return cl, nil
	}

	return nil, fmt.Errorf("cluster %s not found", clusterName)
}

// Run starts the provider and blocks, watching for kubeconfig secrets.
// It implements the Provider interface.
func (p *KubeconfigProvider) Run(ctx context.Context, mgr KubeClusterManager) error {
	p.log.Info("starting kubeconfig provider", "namespace", p.opts.Namespace, "label", p.opts.KubeconfigLabel)

	// Set the manager
	p.SetManager(mgr)

	// Wait for the controller-runtime cache to be ready before using it
	if mgr != nil && mgr.GetCache() != nil {
		p.log.Info("Waiting for controller-runtime cache to be ready")
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return fmt.Errorf("timed out waiting for cache to sync")
		}
		p.log.Info("Controller-runtime cache is synced")
	} else {
		p.log.Info("No manager or cache available, skipping cache sync")
	}

	// Do initial sync using direct API call (not cached client)
	if err := p.syncSecretsInternal(ctx); err != nil {
		p.log.Error(err, "initial secret sync failed", "error", err.Error())
		// Continue anyway - don't exit on sync failure
	} else {
		p.log.Info("Initial secret sync successful")
	}

	// Create a Kubernetes clientset for watching
	var config *rest.Config
	var err error

	// First, try to get the config from controller-runtime
	config, err = rest.InClusterConfig()
	if err != nil {
		p.log.Info("not running in-cluster, using kubeconfig for local development")

		// Look for kubeconfig in default locations
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		rules.DefaultClientConfig = &clientcmd.DefaultClientConfig

		clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
		config, err = clientConfig.ClientConfig()
		if err != nil {
			return fmt.Errorf("failed to create config: %w", err)
		}
	}

	p.log.Info("successfully connected to kubernetes api", "host", config.Host)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Set up label selector for our kubeconfig label
	labelSelector := fmt.Sprintf("%s=true", p.opts.KubeconfigLabel)
	p.log.Info("watching for kubeconfig secrets", "selector", labelSelector)

	// Watch for secret changes
	go p.watchSecrets(ctx, clientset, labelSelector)

	<-ctx.Done()
	p.log.Info("Provider context cancelled, shutting down gracefully")
	return context.Canceled
}

// Reconcile implements the controller-runtime Reconciler interface
func (p *KubeconfigProvider) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := p.log.WithValues("secret", req.NamespacedName)
	secret := &corev1.Secret{}

	if err := p.Client.Get(ctx, req.NamespacedName, secret); err != nil {
		if errors.IsNotFound(err) {
			log.Info("secret not found, handling deletion")
			// Secret was deleted, handle deletion
			p.handleSecretDelete(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      req.Name,
					Namespace: req.Namespace,
				},
			})
			return reconcile.Result{}, nil
		}
		log.Error(err, "failed to get secret")
		return reconcile.Result{}, fmt.Errorf("failed to get secret: %w", err)
	}

	log.Info("processing secret")
	// Handle secret update/creation
	p.handleSecretUpsert(ctx, secret)

	return reconcile.Result{}, nil
}

// IndexField indexes a field on all clusters, existing and future.
// It implements the Provider interface.
func (p *KubeconfigProvider) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	// Save for future clusters
	p.indexers = append(p.indexers, index{
		object:       obj,
		field:        field,
		extractValue: extractValue,
	})

	// Apply to existing clusters
	for name, cl := range p.clusters {
		if err := cl.GetCache().IndexField(ctx, obj, field, extractValue); err != nil {
			return fmt.Errorf("failed to index field %q on cluster %q: %w", field, name, err)
		}
	}

	return nil
}

// Engage creates, starts and registers a new cluster with the manager
func (p *KubeconfigProvider) Engage(ctx context.Context, clusterName string, config *rest.Config) error {
	log := p.log.WithValues("cluster", clusterName)
	log.Info("Creating new controller-runtime cluster")

	// Add timeout to the config
	config.Timeout = p.opts.ConnectionTimeout

	// Create a new cluster
	cl, err := cluster.New(config, func(o *cluster.Options) {
		o.Scheme = p.opts.Scheme
		// Set a longer cache sync timeout
		o.Cache.SyncPeriod = &p.opts.CacheSyncTimeout
	})
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	// Create a new context for this cluster
	clusterCtx, cancel := context.WithCancel(ctx)

	// Start the cluster in a goroutine
	go func() {
		if err := cl.Start(clusterCtx); err != nil {
			log.Error(err, "Failed to start cluster")
		}
	}()

	// Wait for cache to sync with a timeout
	syncCtx, syncCancel := context.WithTimeout(ctx, p.opts.CacheSyncTimeout)
	defer syncCancel()

	log.Info("Waiting for cluster cache to sync", "timeout", p.opts.CacheSyncTimeout)
	if !cl.GetCache().WaitForCacheSync(syncCtx) {
		cancel() // Clean up if sync fails
		return fmt.Errorf("timed out waiting for cache to sync for cluster %s", clusterName)
	}
	log.Info("Cluster cache successfully synced")

	// Register the cluster with the manager if available
	p.lock.RLock()
	manager := p.manager
	p.lock.RUnlock()

	if manager != nil {
		if err := manager.Engage(ctx, clusterName, cl); err != nil {
			cancel() // Clean up if registration fails
			return fmt.Errorf("failed to register cluster: %w", err)
		}
	} else {
		log.Info("No manager available, skipping registration with manager")
	}

	// Register the cluster in our internal state
	p.lock.Lock()
	p.clusters[clusterName] = cl
	p.cancelFns[clusterName] = cancel
	p.lock.Unlock()

	// Apply any pending indexers
	for _, idx := range p.indexers {
		if err := cl.GetCache().IndexField(ctx, idx.object, idx.field, idx.extractValue); err != nil {
			return fmt.Errorf("failed to index field %q on cluster %q: %w", idx.field, clusterName, err)
		}
	}

	log.Info("Successfully engaged cluster")
	return nil
}

// handleSecretUpsert handles the addition or update of a kubeconfig secret
func (p *KubeconfigProvider) handleSecretUpsert(ctx context.Context, secret *corev1.Secret) {
	log := p.log.WithValues("secret", types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace})
	log.Info("Processing kubeconfig secret")

	clusterName := secret.Name

	// Check if we already have this cluster
	p.lock.RLock()
	_, exists := p.clusters[clusterName]
	existingCancelFn := p.cancelFns[clusterName]
	p.lock.RUnlock()

	// Get kubeconfig from secret
	kubeconfigData, ok := secret.Data[p.opts.KubeconfigKey]
	if !ok || len(kubeconfigData) == 0 {
		log.Error(nil, "Kubeconfig key not found or empty", "key", p.opts.KubeconfigKey)
		return
	}

	log.Info("Found kubeconfig data in secret", "dataSize", len(kubeconfigData))

	// Parse kubeconfig and create REST config
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		log.Error(err, "Failed to parse kubeconfig")
		return
	}

	// Test connection to API server
	log.Info("Testing connection to API server", "host", restConfig.Host)
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Error(err, "Failed to create clientset from config")
		return
	}

	// Attempt to list nodes as a basic connectivity test
	_, err = clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		log.Error(err, "Failed to connect to Kubernetes API server", "host", restConfig.Host)
		return
	}
	log.Info("Successfully connected to API server", "host", restConfig.Host)

	// If cluster exists and it's an update, we need to stop the old one
	if exists {
		log.Info("Updating existing cluster")
		if existingCancelFn != nil {
			existingCancelFn()
		}
		p.lock.Lock()
		delete(p.clusters, clusterName)
		delete(p.cancelFns, clusterName)
		p.lock.Unlock()
	}

	// Create and start cluster
	if err := p.Engage(ctx, clusterName, restConfig); err != nil {
		log.Error(err, "Failed to engage cluster")
		return
	}

	// Log current cluster count
	p.lock.RLock()
	clusterCount := len(p.clusters)
	p.lock.RUnlock()
	log.Info("Currently managing clusters", "count", clusterCount)
}

// Disengage stops and removes a cluster from the provider
func (p *KubeconfigProvider) Disengage(ctx context.Context, clusterName string) error {
	log := p.log.WithValues("cluster", clusterName)
	log.Info("Disengaging cluster")

	// Find the cluster and cancel function
	p.lock.RLock()
	_, exists := p.clusters[clusterName]
	if !exists {
		p.lock.RUnlock()
		return fmt.Errorf("cluster %s not found", clusterName)
	}

	// Get the cancel function
	cancelFn, exists := p.cancelFns[clusterName]
	if !exists {
		p.lock.RUnlock()
		return fmt.Errorf("cancel function for cluster %s not found", clusterName)
	}

	// Get manager reference while holding the read lock
	mgr := p.manager
	p.lock.RUnlock()

	// Disengage from manager if available
	if mgr != nil {
		if err := mgr.Disengage(ctx, clusterName); err != nil {
			log.Error(err, "Failed to disengage from manager")
			// Continue with cleanup even if manager disengage fails
		}
	}

	// Stop the cluster
	cancelFn()

	// Clean up our maps
	p.lock.Lock()
	delete(p.clusters, clusterName)
	delete(p.cancelFns, clusterName)
	p.lock.Unlock()

	log.Info("Successfully disengaged cluster")
	return nil
}

// handleSecretDelete handles the deletion of a kubeconfig secret
func (p *KubeconfigProvider) handleSecretDelete(secret *corev1.Secret) {
	log := p.log.WithValues("secret", types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace})
	log.Info("Handling kubeconfig secret deletion")

	clusterName := secret.Name

	// Use Disengage to handle cleanup
	if err := p.Disengage(context.Background(), clusterName); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			log.Error(err, "Failed to disengage cluster")
		}
		return
	}

	// Log current cluster count
	p.lock.RLock()
	clusterCount := len(p.clusters)
	p.lock.RUnlock()
	log.Info("Currently managing clusters", "count", clusterCount)
}

// watchSecrets sets up a watch for secret changes
func (p *KubeconfigProvider) watchSecrets(ctx context.Context, clientset *kubernetes.Clientset, labelSelector string) {
	p.log.Info("Starting watcher for secrets with selector", "selector", labelSelector)

	for {
		select {
		case <-ctx.Done():
			p.log.Info("Secret watcher context cancelled, shutting down")
			return
		default:
			// Continue with watch
		}

		watcher, err := clientset.CoreV1().Secrets(p.opts.Namespace).Watch(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})

		if err != nil {
			// Handle context cancellation - normal shutdown
			if stderrors.Is(err, context.Canceled) {
				p.log.Info("Context cancelled while creating watcher, shutting down")
				return
			}

			p.log.Error(err, "Error creating watcher")
			time.Sleep(5 * time.Second)
			continue
		}

		p.log.Info("Successfully created watcher")

		// Process events until the channel is closed or context is cancelled
		for {
			select {
			case <-ctx.Done():
				p.log.Info("Context cancelled, closing watcher")
				watcher.Stop()
				return
			case event, ok := <-watcher.ResultChan():
				if !ok {
					p.log.Info("Watch channel closed, retrying")
					time.Sleep(1 * time.Second)
					break
				}

				// Process the event
				secret, ok := event.Object.(*corev1.Secret)
				if !ok {
					p.log.Error(nil, "Expected secret", "type", fmt.Sprintf("%T", event.Object))
					continue
				}

				// Check if we've already seen this exact version of the secret
				key := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
				p.lock.RLock()
				seenVersion, alreadySeen := p.seenHashes[key]
				p.lock.RUnlock()

				if alreadySeen && seenVersion == secret.ResourceVersion {
					// We've already processed this exact version of the secret
					p.log.V(1).Info("Skipping already processed secret version",
						"secret", key,
						"resourceVersion", secret.ResourceVersion)
					continue
				}

				// Process the secret according to event type
				switch event.Type {
				case watch.Added, watch.Modified:
					p.handleSecretUpsert(ctx, secret)
					// Update the seen hash to avoid reprocessing
					p.lock.Lock()
					p.seenHashes[key] = secret.ResourceVersion
					p.lock.Unlock()
				case watch.Deleted:
					p.handleSecretDelete(secret)
					// Remove from seen hashes
					p.lock.Lock()
					delete(p.seenHashes, key)
					p.lock.Unlock()
				}
			}
		}
	}
}

// SyncSecrets provides a public method to manually trigger a sync of all kubeconfig secrets
// This is exposed for testing or forced refreshes, but normal operation uses the internal
// sync during Run() initialization
func (p *KubeconfigProvider) SyncSecrets(ctx context.Context) error {
	return p.syncSecretsInternal(ctx)
}

// syncSecretsInternal lists all matching secrets and processes them
// This is now a private implementation method used by both Run and the public SyncSecrets
func (p *KubeconfigProvider) syncSecretsInternal(ctx context.Context) error {
	// Create a direct Kubernetes clientset instead of using the cached client
	config, err := rest.InClusterConfig()
	if err != nil {
		// Not in cluster, try using default kubeconfig
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		config, err = kubeConfig.ClientConfig()
		if err != nil {
			return fmt.Errorf("failed to create config: %w", err)
		}
	}

	// Create a clientset for direct API access
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Use the clientset to list secrets directly (bypassing cache)
	secretList, err := clientset.CoreV1().Secrets(p.opts.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", p.opts.KubeconfigLabel),
	})
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	p.log.Info("Found secrets with kubeconfig label", "count", len(secretList.Items))

	// Process existing secrets
	currentKeys := make(map[string]bool)
	for i := range secretList.Items {
		secret := &secretList.Items[i]
		key := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
		currentKeys[key] = true

		// Check if this is a new or updated secret
		if hash, exists := p.seenHashes[key]; !exists || hash != secret.ResourceVersion {
			p.handleSecretUpsert(ctx, secret)
			p.seenHashes[key] = secret.ResourceVersion
		}
	}

	// Check for deleted secrets
	p.lock.RLock()
	for name := range p.clusters {
		key := fmt.Sprintf("%s/%s", p.opts.Namespace, name)
		if _, exists := currentKeys[key]; !exists {
			// This secret has been deleted
			p.lock.RUnlock()
			p.handleSecretDelete(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: p.opts.Namespace,
				},
			})
			p.lock.RLock()
			// Remove from seen hashes
			delete(p.seenHashes, key)
		}
	}
	p.lock.RUnlock()

	return nil
}

// SetManager explicitly sets the manager for the provider
// This should be called before any other operations
func (p *KubeconfigProvider) SetManager(mgr KubeClusterManager) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.manager = mgr
	p.log.Info("Manager explicitly set for provider")
}
