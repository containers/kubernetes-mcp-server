package kubernetes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
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
	// caMaxRedirects bounds redirect hops from the CA endpoint. A CA is a
	// trust root fetched on each refresh; any real deployment serves it
	// directly, and a long chain is a redirect-loop or abuse signal.
	caMaxRedirects = 3
	// caFetchMaxBackoff is the cumulative 1s+2s... backoff between retries.
	caFetchMaxBackoff = time.Duration(caFetchAttempts*(caFetchAttempts-1)/2) * time.Second
	// caFetchMaxDuration bounds a whole fetch with retries: caFetchAttempts
	// timeout-bounded attempts plus the backoff between them. Each attempt
	// gets its own caFetchTimeout (see fetchCA), so a hung first attempt
	// cannot consume the retry budget.
	caFetchMaxDuration = caFetchTimeout*caFetchAttempts + caFetchMaxBackoff
)

// errCARedirectPolicy marks a redirect rejected by the CA client's redirect
// policy (non-https hop or too many hops). Rejections are deterministic, so
// they must not be retried.
var errCARedirectPolicy = errors.New("caURL redirect rejected")

// CAFetchClientFactory builds the HTTP client used to fetch CA certificates
// from caURL endpoints. It is a variable so tests can pin a TLS test
// server's certificate without touching the process-wide system pool.
// The client verifies caURL against the system trust store (system roots or
// SSL_CERT_FILE), not against the cluster CA being fetched; the redirect
// policy applies only to clients built by this factory, so callers that
// replace the factory inherit the responsibility of rejecting non-https
// redirects.
var CAFetchClientFactory = func() *http.Client {
	return &http.Client{
		Timeout: caFetchTimeout,
		// A hostile or misconfigured CA host could redirect the fetch to
		// cleartext, letting a network attacker substitute a CA. Follow
		// only https hops, and cap the chain.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= caMaxRedirects {
				return fmt.Errorf("%w: too many redirects (max %d)", errCARedirectPolicy, caMaxRedirects)
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("%w: redirect target must be https", errCARedirectPolicy)
			}
			return nil
		},
	}
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

// caCacheRoot is the base directory cached CA certificates live under.
// It is a variable (defaulting to the system temp dir, which honors
// $TMPDIR) so tests can redirect the cache away from the shared system
// temp dir; production never changes it. Operators with a read-only root
// filesystem point TMPDIR at writable storage or mount an emptyDir at
// /tmp.
var caCacheRoot = os.TempDir()

func caCacheFilePath(caURL string) string {
	sum := sha256.Sum256([]byte(caURL))
	return filepath.Join(caCacheRoot, caCacheSubdir, hex.EncodeToString(sum[:8])+".crt")
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
	// client-go rejects a CAFile combined with insecure-skip-tls-verify
	// ("specifying a root certificates file with the insecure flag is not
	// allowed"), so fail here with a message that names the conflict
	// instead of surfacing that cryptic error later, after the fetch.
	if restConfig.Insecure {
		return nil, fmt.Errorf("cluster %q sets insecure-skip-tls-verify and serves its CA via caURL; remove one of them", contextName)
	}

	cacheFile := caCacheFilePath(caURL)
	client := CAFetchClientFactory()
	// Bounds the whole fetch-with-retries so manager creation cannot hang
	// for longer than the attempt-and-backoff envelope (see fetchCA).
	fetchCtx, cancel := context.WithTimeout(ctx, caFetchMaxDuration)
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
		logger.V(2).Info("CA auto-refresh disabled; cached CA refreshes on SIGHUP or when a manager is rebuilt", "context", contextName)
	}
	// The refresher is kept even when auto-refresh is disabled so SIGHUP
	// reload (Manager.RefreshCAs) can still fetch on demand; its background
	// loop just never starts (see caRefresher.start).
	return newCARefresher(ctx, cacheFile, caURL, client, interval, logger), nil
}

// fetchCA downloads the CA served at caURL, retrying transient failures.
func fetchCA(ctx context.Context, client *http.Client, caURL string) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		// Bound each attempt individually so a hung first attempt cannot
		// consume the retry budget; the caller's outer deadline (see
		// caFetchMaxDuration) still caps the whole fetch-with-retries.
		attemptCtx, cancel := context.WithTimeout(ctx, caFetchTimeout)
		data, retryable, err := fetchCAOnce(attemptCtx, client, caURL)
		cancel()
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
		// Redirect-policy rejections are deterministic; retrying them only
		// burns the retry budget.
		return nil, !errors.Is(err, errCARedirectPolicy), err
	}
	defer func() { _ = resp.Body.Close() }()
	// Clients built outside CAFetchClientFactory (tests, callers that
	// replaced it) may follow redirects without a policy; re-check the
	// final URL so a scheme downgrade (https to http) is still caught.
	startURL, err := url.Parse(caURL)
	if err == nil && resp.Request != nil && resp.Request.URL.Scheme != startURL.Scheme {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, false, fmt.Errorf("CA URL %s redirected from %s to %s", caURL, startURL.Scheme, resp.Request.URL.Scheme)
	}
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
	// certutil.ParseCertsPEM validates the whole bundle: every CERTIFICATE
	// block parses, and at least one is required.
	served, err := certutil.ParseCertsPEM(body)
	if err != nil || len(served) == 0 {
		return nil, false, fmt.Errorf("CA URL %s response is not a PEM CA certificate: %w", caURL, err)
	}
	return body, false, nil
}

// ensurePrivateCacheDir makes dir exist and be private before CA files
// go there. MkdirAll's mode only applies when it creates the directory,
// so a predictable path beneath the shared system temp dir must be
// checked rather than trusted: another local user could otherwise
// pre-create it writable or as a symlink and substitute their own
// certificate. Fails closed on symlinks, group/world writable
// directories, and foreign ownership. World-readable is tolerated: the
// CA files themselves are 0600, and read access cannot swap a trust
// anchor.
func ensurePrivateCacheDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create CA cache directory %s: %w", dir, err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("stat CA cache directory %s: %w", dir, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("CA cache directory %s must not be a symlink", dir)
	}
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return fmt.Errorf("CA cache directory %s is group or world writable (mode %#o)", dir, perm)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("CA cache directory %s is owned by uid %d, not the current user", dir, st.Uid)
	}
	return nil
}

// writeCAFile atomically writes data to path so readers (client-go's
// CA-file rotation) never observe a partially written certificate.
func writeCAFile(path string, data []byte) error {
	if err := ensurePrivateCacheDir(filepath.Dir(path)); err != nil {
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
	// A zero interval disables the background loop entirely; the refresher
	// then only fetches on demand (Manager.RefreshCAs on SIGHUP). Close
	// sees started=false and skips waiting on a loop that never ran.
	if r.interval <= 0 {
		return
	}
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
	// shutdown cancel the fetch; caFetchMaxDuration below still bounds a
	// fetch-with-retries.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.ctx), caFetchMaxDuration)
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
