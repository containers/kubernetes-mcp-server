package clients

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// ClusterClient wraps a Kubernetes client with metadata
type ClusterClient struct {
	Name      string
	Clientset kubernetes.Interface
	Config    *rest.Config
	Context   string
}

// Registry manages multiple Kubernetes cluster clients
type Registry struct {
	clients map[string]*ClusterClient
	mu      sync.RWMutex
	timeout time.Duration
}

// NewRegistry creates a new client registry
func NewRegistry() *Registry {
	return &Registry{
		clients: make(map[string]*ClusterClient),
		timeout: 30 * time.Second,
	}
}

// SetTimeout sets the default timeout for cluster operations
func (r *Registry) SetTimeout(timeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timeout = timeout
}

// RegisterInCluster registers the in-cluster configuration
func (r *Registry) RegisterInCluster() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	r.clients["in-cluster"] = &ClusterClient{
		Name:      "in-cluster",
		Clientset: clientset,
		Config:    config,
		Context:   "in-cluster",
	}

	return nil
}

// RegisterFromKubeconfig registers clients from a kubeconfig file
func (r *Registry) RegisterFromKubeconfig(kubeconfigPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Load kubeconfig
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Register each context
	for contextName, context := range config.Contexts {
		if err := r.registerContext(config, contextName, context); err != nil {
			// Log error but continue with other contexts
			continue
		}
	}

	return nil
}

// RegisterContext registers a specific context from kubeconfig
func (r *Registry) RegisterContext(kubeconfigPath, contextName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	context, exists := config.Contexts[contextName]
	if !exists {
		return fmt.Errorf("context %s not found in kubeconfig", contextName)
	}

	return r.registerContext(config, contextName, context)
}

// registerContext is an internal helper to register a context
func (r *Registry) registerContext(config *api.Config, contextName string, context *api.Context) error {
	// Build client config for this context
	clientConfig := clientcmd.NewNonInteractiveClientConfig(
		*config,
		contextName,
		&clientcmd.ConfigOverrides{},
		nil,
	)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to create client config for context %s: %w", contextName, err)
	}

	// Set timeout
	restConfig.Timeout = r.timeout

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create clientset for context %s: %w", contextName, err)
	}

	r.clients[contextName] = &ClusterClient{
		Name:      contextName,
		Clientset: clientset,
		Config:    restConfig,
		Context:   contextName,
	}

	return nil
}

// GetClient returns a client for the specified cluster
func (r *Registry) GetClient(clusterName string) (*ClusterClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, exists := r.clients[clusterName]
	if !exists {
		return nil, fmt.Errorf("cluster %s not found in registry", clusterName)
	}

	return client, nil
}

// GetAllClients returns all registered clients
func (r *Registry) GetAllClients() map[string]*ClusterClient {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	clients := make(map[string]*ClusterClient, len(r.clients))
	for name, client := range r.clients {
		clients[name] = client
	}

	return clients
}

// ListClusterNames returns names of all registered clusters
func (r *Registry) ListClusterNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}

	return names
}

// HasCluster checks if a cluster is registered
func (r *Registry) HasCluster(clusterName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.clients[clusterName]
	return exists
}

// UnregisterCluster removes a cluster from the registry
func (r *Registry) UnregisterCluster(clusterName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, clusterName)
}

// Clear removes all registered clients
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clients = make(map[string]*ClusterClient)
}

// ExecuteOnCluster executes a function on a specific cluster with timeout
func (r *Registry) ExecuteOnCluster(ctx context.Context, clusterName string, fn func(*ClusterClient) (interface{}, error)) (interface{}, error) {
	client, err := r.GetClient(clusterName)
	if err != nil {
		return nil, err
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// Execute function
	resultChan := make(chan interface{}, 1)
	errorChan := make(chan error, 1)

	go func() {
		result, err := fn(client)
		if err != nil {
			errorChan <- err
			return
		}
		resultChan <- result
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("operation timed out for cluster %s", clusterName)
	}
}

// ExecuteOnAllClusters executes a function on all clusters concurrently
func (r *Registry) ExecuteOnAllClusters(ctx context.Context, fn func(*ClusterClient) (interface{}, error)) map[string]ClusterResult {
	clients := r.GetAllClients()
	results := make(map[string]ClusterResult, len(clients))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, client := range clients {
		wg.Add(1)
		go func(clusterName string, clusterClient *ClusterClient) {
			defer wg.Done()

			result, err := r.ExecuteOnCluster(ctx, clusterName, fn)

			mu.Lock()
			results[clusterName] = ClusterResult{
				ClusterName: clusterName,
				Result:      result,
				Error:       err,
			}
			mu.Unlock()
		}(name, client)
	}

	wg.Wait()
	return results
}

// ClusterResult represents the result of an operation on a cluster
type ClusterResult struct {
	ClusterName string
	Result      interface{}
	Error       error
}


// Global registry instance (singleton pattern for simplicity)
var (
	globalRegistry     *Registry
	globalRegistryOnce sync.Once
	globalRegistryMu   sync.Mutex
)

// GetOrCreateRegistry returns the global registry, creating it if needed
// It initializes with the provided Kubernetes client if this is the first call
func GetOrCreateRegistry(k8sClient interface{}) *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewRegistry()
		// Try to register from default kubeconfig
		// This is best-effort and won't fail if kubeconfig is not available
		_ = globalRegistry.RegisterFromKubeconfig(clientcmd.RecommendedHomeFile)
	})
	return globalRegistry
}

// ResetGlobalRegistry resets the global registry (useful for testing)
func ResetGlobalRegistry() {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()
	globalRegistry = nil
	globalRegistryOnce = sync.Once{}
}

// Made with Bob
// Made with Bob
