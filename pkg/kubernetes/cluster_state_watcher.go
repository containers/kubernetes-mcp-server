package kubernetes

import (
	"context"

	"k8s.io/client-go/rest"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
)

// newClusterStateWatcher returns the cluster state watcher for k, or nil when
// the server has no identity of its own to poll the cluster with.
//
// With cluster_auth_mode explicitly set to passthrough and a kubeconfig that
// carries no credentials, every request brings its own token, so a background
// poll can only reach the API server anonymously and gets refused. Skip the
// watcher in that case. When cluster_auth_mode is unset, the kubeconfig may
// still be usable without credentials (for example through kubectl proxy), so
// the watcher keeps running.
func newClusterStateWatcher(ctx context.Context, cfg *config.Config, k *Kubernetes) watcher.Watcher {
	if cfg.ClusterAuthMode.Get() == config.ClusterAuthPassthrough && !hasClientCredentials(k.RESTConfig()) {
		klogutil.FromContext(ctx).V(1).Info("Cluster state watcher disabled: cluster_auth_mode is passthrough and the kubeconfig has no credentials")
		return nil
	}
	return watcher.NewClusterState(ctx, k.DiscoveryClient(), cfg.ClusterStatePollInterval.Get(), cfg.ClusterStateDebounceWindow.Get())
}

// hasClientCredentials reports whether restConfig authenticates the client in
// any way that client-go supports.
func hasClientCredentials(restConfig *rest.Config) bool {
	return restConfig.BearerToken != "" ||
		restConfig.BearerTokenFile != "" ||
		restConfig.Username != "" ||
		restConfig.Password != "" ||
		len(restConfig.CertData) > 0 ||
		restConfig.CertFile != "" ||
		restConfig.ExecProvider != nil ||
		restConfig.AuthProvider != nil
}
