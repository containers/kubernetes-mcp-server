package kubernetes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

// A cluster's API-server CA can be served over HTTPS (e.g. an endpoint that
// mirrors the cluster CA for cross-cluster consumers). A kubeconfig cluster
// entry opts into this with the kubernetes-mcp-server extension:
//
//	clusters:
//	  - name: prod
//	    cluster:
//	      server: https://kubernetes.default.svc
//	      extensions:
//	        - name: kubernetes-mcp-server
//	          extension:
//	            caURL: https://ca.example.com/
//
// The CA is fetched into a cache file and the Kubernetes client is pointed at
// that file (rest.Config TLSClientConfig.CAFile). client-go reloads CA files
// it is pointed at and swaps the TLS transport when the content changes, so a
// rotated CA converges without a restart. A background refresher keeps the
// cache file in sync with the served CA.

const (
	// caExtensionName is the kubeconfig cluster extension key carrying caURL.
	caExtensionName = "kubernetes-mcp-server"
	// caCacheSubdir is the default cache directory below the system temp dir.
	caCacheSubdir = "kubernetes-mcp-server-ca"
	// caFetchTimeout bounds each CA fetch so a hung endpoint cannot stall
	// manager creation.
	caFetchTimeout = 15 * time.Second
	// caFetchAttempts bounds transient failures (first DNS lookup from a
	// fresh pod can race the network).
	caFetchAttempts = 3
	// caMaxBodyBytes bounds CA response size; bundles are a few KB.
	caMaxBodyBytes = 1 << 20 // 1 MiB
)

// CAFetchClientFactory builds the HTTP client used to fetch CA certificates
// from caURL endpoints. It is a variable so tests can pin a TLS test
// server's certificate without touching the process-wide system pool.
var CAFetchClientFactory = func() *http.Client {
	return &http.Client{Timeout: caFetchTimeout}
}

func caURLFromClusterExtensions(cluster *clientcmdapi.Cluster) string {
	obj, ok := cluster.Extensions[caExtensionName]
	if !ok || obj == nil {
		return ""
	}
	// Extensions decode to runtime.Object; concrete shapes vary by codec
	// (RawExtension, unstructured), so JSON round-trip covers all of them.
	data, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	var ext struct {
		CAURL string `json:"caURL"`
	}
	if err := json.Unmarshal(data, &ext); err != nil {
		return ""
	}
	return ext.CAURL
}

func validateCAURL(caURL string) error {
	u, err := url.Parse(caURL)
	if err != nil {
		return fmt.Errorf("invalid caURL %q: %w", caURL, err)
	}
	// A CA is a trust root; fetching it over cleartext would let a
	// network attacker substitute their own CA and then impersonate the
	// cluster the CA is meant to authenticate.
	if u.Scheme != "https" {
		return fmt.Errorf("caURL %q must be an https URL", caURL)
	}
	if u.Host == "" {
		return fmt.Errorf("caURL %q has no host", caURL)
	}
	return nil
}

func caCacheDir(configured string) string {
	if configured != "" {
		return configured
	}
	return filepath.Join(os.TempDir(), caCacheSubdir)
}

func caCacheFilePath(cacheDir, caURL string) string {
	sum := sha256.Sum256([]byte(caURL))
	return filepath.Join(cacheDir, hex.EncodeToString(sum[:8])+".crt")
}

// applyClusterCAURL points restConfig's TLS at a cached copy of the CA served
// by the resolved context's cluster caURL extension. It returns a refresher
// that keeps the cache file in sync, or nil when the cluster has no caURL.
func applyClusterCAURL(
	ctx context.Context,
	config api.BaseConfig,
	rawConfig *clientcmdapi.Config,
	contextName string,
	restConfig *rest.Config,
) (*caRefresher, error) {
	logger := klogutil.FromContext(ctx)
	if rawConfig == nil || restConfig == nil {
		return nil, nil
	}
	contextInfo, ok := rawConfig.Contexts[contextName]
	if !ok || contextInfo == nil {
		return nil, nil
	}
	cluster, ok := rawConfig.Clusters[contextInfo.Cluster]
	if !ok || cluster == nil {
		return nil, nil
	}
	caURL := caURLFromClusterExtensions(cluster)
	if caURL == "" {
		return nil, nil
	}
	if err := validateCAURL(caURL); err != nil {
		return nil, err
	}

	cacheFile := caCacheFilePath(caCacheDir(config.GetCACacheDir()), caURL)
	client := CAFetchClientFactory()
	fetchCtx, cancel := context.WithTimeout(ctx, caFetchTimeout)
	defer cancel()
	data, err := fetchCA(fetchCtx, client, caURL)
	if err != nil {
		return nil, fmt.Errorf("fetch CA for context %q from %s: %w", contextName, caURL, err)
	}
	if err := writeCAFile(cacheFile, data); err != nil {
		return nil, fmt.Errorf("cache CA for context %q: %w", contextName, err)
	}
	logger.V(2).Info("Using CA fetched from kubeconfig extension", "context", contextName, "ca_url", caURL, "ca_file", cacheFile)

	// The fetched CA wins over any certificate-authority-data so the cached
	// file is the single source client-go watches for rotation.
	restConfig.CAFile = cacheFile
	restConfig.CAData = nil

	interval := config.GetCARefreshInterval()
	if interval <= 0 {
		logger.V(2).Info("CA refresh disabled; cached CA only refreshes when a manager is rebuilt", "context", contextName)
		return nil, nil
	}
	return newCARefresher(ctx, cacheFile, caURL, client, interval, logger), nil
}

// fetchCA downloads the CA served at caURL, retrying transient failures.
func fetchCA(ctx context.Context, client *http.Client, caURL string) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		data, retryable, err := fetchCAOnce(ctx, client, caURL)
		if err == nil {
			return data, nil
		}
		if !retryable || attempt >= caFetchAttempts {
			return nil, err
		}
		select {
		case <-time.After(time.Duration(attempt) * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func fetchCAOnce(ctx context.Context, client *http.Client, caURL string) (data []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, caURL, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		// Permanent client errors never become retryable; transient 5xx and
		// 429 responses and network errors are worth a retry.
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retryable, fmt.Errorf("CA URL %s returned status %d", caURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, caMaxBodyBytes+1))
	if err != nil {
		return nil, true, err
	}
	if len(body) > caMaxBodyBytes {
		return nil, false, fmt.Errorf("CA URL %s response exceeds %d bytes", caURL, caMaxBodyBytes)
	}
	block, _ := pem.Decode(body)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, false, fmt.Errorf("CA URL %s response is not a PEM CA certificate", caURL)
	}
	// AppendCertsFromPEM validates the whole bundle (it handles multi-block
	// PEM where ParseCertificates expects raw DER).
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(body) {
		return nil, false, fmt.Errorf("CA URL %s response is not a valid CA certificate bundle", caURL)
	}
	return body, false, nil
}

// writeCAFile atomically writes data to path so readers (client-go's CA-file
// rotation) never observe a partially written certificate.
func writeCAFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ca-*.tmp")
	if err != nil {
		return err
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// caRefresher re-fetches a cluster's CA on an interval so a rotated CA (e.g.
// after the cluster is re-provisioned) lands in the cache file client-go is
// pointed at; client-go then swaps to the new CA within its own refresh cycle.
type caRefresher struct {
	ctx       context.Context
	cacheFile string
	caURL     string
	client    *http.Client
	interval  time.Duration
	logger    klog.Logger

	// started records whether the refresh loop was launched so Close can
	// skip waiting on a loop that never ran (a refresher may be discarded
	// before start without a goroutine existing to stop).
	started   atomic.Bool
	stopCh    chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
}

func newCARefresher(
	ctx context.Context,
	cacheFile, caURL string,
	client *http.Client,
	interval time.Duration,
	logger klog.Logger,
) *caRefresher {
	return &caRefresher{
		ctx:       ctx,
		cacheFile: cacheFile,
		caURL:     caURL,
		client:    client,
		interval:  interval,
		logger:    logger,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

func (r *caRefresher) start() {
	// NewKubeconfigManager is the only caller and starts the loop only
	// after the manager is fully built, so a construction error leaves no
	// goroutine behind that would need stopping.
	r.started.Store(true)
	go r.loop()
}

func (r *caRefresher) loop() {
	defer close(r.doneCh)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.refreshOnce()
		case <-r.stopCh:
			return
		}
	}
}

func (r *caRefresher) refreshOnce() {
	// WithoutCancel keeps the caller's log/trace values without letting a
	// shutdown cancel the fetch; the timeout below still bounds it.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.ctx), caFetchTimeout)
	defer cancel()
	data, err := fetchCA(ctx, r.client, r.caURL)
	if err != nil {
		// Keep the previous CA; it worked until now and a bad transient
		// refresh must not break a healthy client.
		r.logger.Error(err, "failed to refresh cached CA, keeping previous CA", "ca_url", r.caURL, "ca_file", r.cacheFile)
		return
	}
	// Skip the rewrite when the served CA is unchanged; pointless temp-file
	// churn and client-go re-reads on every tick.
	if current, err := os.ReadFile(r.cacheFile); err == nil && bytes.Equal(current, data) {
		return
	}
	if err := writeCAFile(r.cacheFile, data); err != nil {
		r.logger.Error(err, "failed to write refreshed CA", "ca_file", r.cacheFile)
	}
}

// Close stops the refresh loop and blocks until it has exited. Safe to call
// multiple times.
func (r *caRefresher) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		if !r.started.Load() {
			return
		}
		close(r.stopCh)
		<-r.doneCh
	})
}
