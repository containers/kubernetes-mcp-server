package kubernetes

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ClusterCACASuite struct {
	suite.Suite
	originalEnv       []string
	originalCacheRoot string
}

func (s *ClusterCACASuite) SetupTest() {
	s.originalEnv = os.Environ()
	// Redirect the CA cache into a per-test temp dir so tests stay
	// hermetic and never touch the shared system temp dir.
	s.originalCacheRoot = caCacheRoot
	caCacheRoot = s.T().TempDir()
}

func (s *ClusterCACASuite) TearDownTest() {
	test.RestoreEnv(s.originalEnv)
	caCacheRoot = s.originalCacheRoot
}

func TestClusterCASuite(t *testing.T) {
	suite.Run(t, new(ClusterCACASuite))
}

// TestWriteCAFileRejectsUnsafeCacheDirs guards the cache directory against
// another local user pre-creating the predictable path beneath the system
// temp dir as writable or as a symlink, then swapping in their own CA.
func (s *ClusterCACASuite) TestWriteCAFileRejectsUnsafeCacheDirs() {
	base := caCacheRoot // per-test temp dir, owned by us

	s.Run("world-writable pre-existing cache dir is rejected", func() {
		if runtime.GOOS == "windows" {
			s.T().Skip("chmod does not expose unix permission bits on windows")
		}
		dir := filepath.Join(base, "world-writable")
		s.Require().NoError(os.MkdirAll(dir, 0o700))
		s.Require().NoError(os.Chmod(dir, 0o777))
		err := writeCAFile(filepath.Join(dir, "ca.crt"), selfSignedPEM())
		s.Require().Error(err)
		s.Contains(err.Error(), "group or world writable")
	})

	s.Run("symlinked cache dir is rejected", func() {
		if runtime.GOOS == "windows" {
			s.T().Skip("creating a symlink on windows needs privileges the test runner does not have")
		}
		target := filepath.Join(base, "elsewhere")
		s.Require().NoError(os.MkdirAll(target, 0o700))
		dir := filepath.Join(base, "symlinked")
		s.Require().NoError(os.Symlink(target, dir))
		err := writeCAFile(filepath.Join(dir, "ca.crt"), selfSignedPEM())
		s.Require().Error(err)
		s.Contains(err.Error(), "must not be a symlink")
	})

	s.Run("private pre-existing cache dir is accepted", func() {
		dir := filepath.Join(base, "private")
		s.Require().NoError(os.MkdirAll(dir, 0o700))
		pemBytes := selfSignedPEM()
		s.Require().NoError(writeCAFile(filepath.Join(dir, "ca.crt"), pemBytes))
		got, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
		s.Require().NoError(err)
		s.Equal(pemBytes, got)
	})

	s.Run("final file symlink is replaced, not followed", func() {
		if runtime.GOOS == "windows" {
			s.T().Skip("creating a symlink on windows needs privileges the test runner does not have")
		}
		dir := filepath.Join(base, "final-symlink")
		s.Require().NoError(os.MkdirAll(dir, 0o700))
		victimPath := filepath.Join(dir, "victim.crt")
		s.Require().NoError(os.WriteFile(victimPath, []byte("victim"), 0o600))
		link := filepath.Join(dir, "ca.crt")
		s.Require().NoError(os.Symlink(victimPath, link))
		s.Require().NoError(writeCAFile(link, selfSignedPEM()))
		got, err := os.ReadFile(victimPath)
		s.Require().NoError(err)
		s.Equal("victim", string(got), "rename must replace the symlink, not write through it")
	})
}

// TestCACacheFileIsPerUser guards the cache directory name: on unix the
// effective uid is part of it, so two users running the server on one host
// neither share a directory nor block each other by pre-creating it.
func (s *ClusterCACASuite) TestCACacheFileIsPerUser() {
	if runtime.GOOS == "windows" {
		s.T().Skip("os.Geteuid returns -1 on windows and the temp dir is already per-user")
	}
	euid := os.Geteuid()
	s.Require().GreaterOrEqual(euid, 0)

	first := caCacheFilePath("https://ca.example.com/")
	second := caCacheFilePath("https://ca.example.com/other.pem")

	dir := filepath.Dir(first)
	s.Equal(caCacheDirName(), filepath.Base(dir))
	s.Equal(fmt.Sprintf("%s-%d", caCacheSubdir, euid), filepath.Base(dir))
	s.Contains(filepath.Base(dir), strconv.Itoa(euid))

	// Distinct caURLs still map to distinct files in the one directory.
	s.Equal(dir, filepath.Dir(second))
	s.NotEqual(first, second)
}

// selfSignedPEM returns a fresh self-signed CA certificate in PEM form, so
// tests never depend on a bundled static certificate.
func selfSignedPEM() []byte {
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// kubeconfigWithCAURL builds a kubeconfig whose cluster carries the
// kubernetes-mcp-server extension pointing at caURL, writes it to disk and
// returns its path.
func kubeconfigWithCAURL(t *testing.T, caURL string) string {
	t.Helper()
	kc := clientcmdapi.NewConfig()
	kc.Clusters["fake"] = &clientcmdapi.Cluster{
		Server: "https://127.0.0.1:1", // never dialed at manager creation
		Extensions: map[string]apiruntime.Object{
			caExtensionName: extensionObject(caURL),
		},
	}
	kc.Contexts["fake"] = &clientcmdapi.Context{Cluster: "fake", AuthInfo: "fake"}
	kc.AuthInfos["fake"] = &clientcmdapi.AuthInfo{Token: "test-token"}
	kc.CurrentContext = "fake"
	return test.KubeconfigFile(t, kc)
}

func extensionObject(caURL string) apiruntime.Object {
	raw, _ := json.Marshal(map[string]string{"caURL": caURL})
	return &apiruntime.Unknown{Raw: raw}
}

func (s *ClusterCACASuite) TestCAURLFromClusterExtensions() {
	s.Run("reads caURL from the kubernetes-mcp-server extension", func() {
		cluster := &clientcmdapi.Cluster{
			Extensions: map[string]apiruntime.Object{
				caExtensionName: extensionObject("https://ca.example.com/"),
			},
		}
		s.Equal("https://ca.example.com/", caURLFromClusterExtensions(cluster))
	})
	s.Run("returns empty without the extension", func() {
		s.Empty(caURLFromClusterExtensions(&clientcmdapi.Cluster{}))
	})
	s.Run("returns empty when the extension is not an object", func() {
		cluster := &clientcmdapi.Cluster{
			Extensions: map[string]apiruntime.Object{
				caExtensionName: &apiruntime.Unknown{Raw: []byte(`"just-a-string"`)},
			},
		}
		s.Empty(caURLFromClusterExtensions(cluster))
	})
	s.Run("survives a write/load round-trip of a real kubeconfig file", func() {
		// The kubeconfig path exercises the actual clientcmd decode of
		// extensions (apiruntime.Unknown payloads).
		path := kubeconfigWithCAURL(s.T(), "https://ca.example.com/")
		cfg := config.New()
		cfg.KubeConfig.SetForTest(path)
		s.Equal("https://ca.example.com/", caURLOfResolvedCluster(s.T(), cfg))
	})
}

func (s *ClusterCACASuite) TestValidateCAURL() {
	s.Run("accepts https", func() {
		s.NoError(validateCAURL("https://ca.example.com/"))
	})
	s.Run("rejects http as a trust root would be interceptable", func() {
		s.Error(validateCAURL("http://ca.example.com/"))
	})
	s.Run("rejects non-http scheme", func() {
		s.Error(validateCAURL("file:///etc/ssl/ca.pem"))
	})
	s.Run("rejects missing host", func() {
		s.Error(validateCAURL("https:///path"))
	})
}

func (s *ClusterCACASuite) TestCAFetchClientRedirectPolicy() {
	s.Run("rejects a redirect to a non-https target", func() {
		client := CAFetchClientFactory()
		httpTarget := &http.Request{URL: &url.URL{Scheme: "http", Host: "ca.example.com"}}
		err := client.CheckRedirect(httpTarget, nil)
		s.Require().Error(err)
		s.ErrorIs(err, errCARedirectPolicy)
	})
	s.Run("caps the redirect chain", func() {
		client := CAFetchClientFactory()
		httpsTarget := &http.Request{URL: &url.URL{Scheme: "https", Host: "ca.example.com"}}
		via := make([]*http.Request, caMaxRedirects)
		s.Require().Error(client.CheckRedirect(httpsTarget, via), "max redirects reached must be rejected")
		s.NoError(client.CheckRedirect(httpsTarget, via[:caMaxRedirects-1]), "still within the cap must be allowed")
	})
}

func (s *ClusterCACASuite) TestFetchCA() {
	s.Run("fetches a PEM certificate", func() {
		want := selfSignedPEM()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(want)
		}))
		defer srv.Close()
		got, err := fetchCA(context.Background(), &http.Client{}, srv.URL)
		s.Require().NoError(err)
		s.Equal(want, got)
	})
	s.Run("errors on non-200 response", func() {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		_, err := fetchCA(context.Background(), &http.Client{}, srv.URL)
		s.Require().Error(err)
		s.ErrorContains(err, "status 404")
	})
	s.Run("fails fast on permanent client errors without retrying", func() {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			http.NotFound(w, nil)
		}))
		defer srv.Close()
		_, err := fetchCA(context.Background(), &http.Client{}, srv.URL)
		s.Require().Error(err)
		s.Equal(int32(1), hits.Load(), "a permanent 404 must not be retried")
	})
	s.Run("retries transient server errors", func() {
		var hits atomic.Int32
		want := selfSignedPEM()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) == 1 {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(want)
		}))
		defer srv.Close()
		got, err := fetchCA(context.Background(), &http.Client{}, srv.URL)
		s.Require().NoError(err)
		s.Equal(want, got)
		s.GreaterOrEqual(hits.Load(), int32(2), "transient 5xx must be retried")
	})
	s.Run("rejects a redirect that downgrades https to http", func() {
		want := selfSignedPEM()
		// The CA endpoint (https) bounces the fetch to a cleartext host.
		httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(want)
		}))
		defer httpSrv.Close()
		tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, httpSrv.URL, http.StatusFound)
		}))
		defer tlsSrv.Close()
		// A client without a redirect policy (simulating a caller that
		// replaced CAFetchClientFactory): the post-Do scheme check must
		// still catch the downgrade.
		pool := x509.NewCertPool()
		pool.AddCert(tlsSrv.Certificate())
		downgradingClient := &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		}
		_, err := fetchCA(context.Background(), downgradingClient, tlsSrv.URL)
		s.Require().Error(err)
		s.ErrorContains(err, "redirected from https to http")
	})
	s.Run("errors on non-PEM response without retrying forever", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not a certificate"))
		}))
		defer srv.Close()
		_, err := fetchCA(context.Background(), &http.Client{}, srv.URL)
		s.Require().Error(err)
		s.ErrorContains(err, "not a PEM CA certificate")
	})
	s.Run("errors on a non-certificate PEM block", func() {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		s.Require().NoError(err)
		keyPEM, err := x509.MarshalECPrivateKey(key)
		s.Require().NoError(err)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyPEM}))
		}))
		defer srv.Close()
		_, err = fetchCA(context.Background(), &http.Client{}, srv.URL)
		s.Require().Error(err)
		s.ErrorContains(err, "not a PEM CA certificate")
	})
}

func (s *ClusterCACASuite) TestRefreshOnceSkipsUnchangedCA() {
	s.Run("does not rewrite an unchanged CA", func() {
		want := selfSignedPEM()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(want)
		}))
		defer srv.Close()
		cacheDir := s.T().TempDir()
		cacheFile := filepath.Join(cacheDir, "ca.crt")
		s.Require().NoError(writeCAFile(cacheFile, want))
		// Backdate the file so a rewrite is detectable via mtime.
		old := time.Now().Add(-time.Hour)
		s.Require().NoError(os.Chtimes(cacheFile, old, old))

		r := newCARefresher(context.Background(), cacheFile, srv.URL, &http.Client{}, time.Minute, klogutil.FromContext(context.Background()))
		defer r.Close()
		r.refreshOnce()

		info, err := os.Stat(cacheFile)
		s.Require().NoError(err)
		s.True(info.ModTime().Before(time.Now().Add(-30*time.Minute)), "an unchanged CA must not be rewritten")

		// No temp files left behind either.
		matches, _ := filepath.Glob(filepath.Join(cacheDir, ".ca-*.tmp"))
		s.Empty(matches)
	})
}

// TestRefreshOnceSerializesConcurrentRefreshes guards the fetch+compare+write
// sequence against a background tick racing an on-demand refresh
// (Manager.RefreshCAs on SIGHUP). Without serialization a slow fetch that
// started earlier could write the CA it fetched on top of a newer CA a faster
// fetch already wrote.
func (s *ClusterCACASuite) TestRefreshOnceSerializesConcurrentRefreshes() {
	const refreshes = 8
	servedCAs := make([][]byte, refreshes)
	for i := range servedCAs {
		servedCAs[i] = selfSignedPEM()
	}
	var (
		mu          sync.Mutex
		inFlight    int
		maxInFlight int
		served      int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		served++
		idx := served
		mu.Unlock()
		// Hold the response so an overlapping fetch would be observed.
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		if idx > len(servedCAs) {
			idx = len(servedCAs)
		}
		_, _ = w.Write(servedCAs[idx-1])
	}))
	defer srv.Close()

	cacheFile := filepath.Join(s.T().TempDir(), "ca.crt")
	r := newCARefresher(context.Background(), cacheFile, srv.URL, &http.Client{}, time.Minute, klogutil.FromContext(context.Background()))
	defer r.Close()

	var wg sync.WaitGroup
	for i := 0; i < refreshes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.refreshOnce()
		}()
	}
	wg.Wait()

	mu.Lock()
	gotMaxInFlight, gotServed := maxInFlight, served
	mu.Unlock()
	s.Equal(1, gotMaxInFlight, "concurrent refreshes must not overlap")
	s.Equal(refreshes, gotServed)
	got, err := os.ReadFile(cacheFile)
	s.Require().NoError(err)
	s.Equal(servedCAs[refreshes-1], got, "the cache must hold the CA from the latest fetch")
}

// TestCloseCancelsInFlightFetch verifies shutdown aborts a fetch that is hung
// on an unresponsive CA endpoint instead of waiting out caFetchMaxDuration.
func (s *ClusterCACASuite) TestCloseCancelsInFlightFetch() {
	entered := make(chan struct{}, 1)
	fetchCancelled := make(chan struct{})
	var cancelOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-r.Context().Done()
		cancelOnce.Do(func() { close(fetchCancelled) })
	}))
	defer srv.Close()

	cacheFile := filepath.Join(s.T().TempDir(), "ca.crt")
	r := newCARefresher(context.Background(), cacheFile, srv.URL, &http.Client{}, time.Millisecond, klogutil.FromContext(context.Background()))
	r.start()
	defer r.Close()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		s.FailNow("the fetch never reached the CA server")
	}

	start := time.Now()
	r.Close()
	s.Less(time.Since(start), 5*time.Second,
		"Close must cancel the in-flight fetch instead of waiting for its own deadline")

	select {
	case <-fetchCancelled:
	case <-time.After(5 * time.Second):
		s.Fail("the in-flight request must observe the cancellation")
	}

	_, err := os.Stat(cacheFile)
	s.ErrorIs(err, os.ErrNotExist, "a cancelled fetch must not write the cache file")
}

func (s *ClusterCACASuite) TestApplyClusterCAURL() {
	s.Run("fetches, caches and refresh the CA over https", func() {
		// Mutable CA server: first CA, then a rotated one. TLS is required
		// for caURL (a trust root must not travel over cleartext), so the
		// fetch client is pointed at the test server's cert via the
		// injectable CAFetchClientFactory; the process-wide system cert pool is
		// deliberately left alone.
		var mu sync.Mutex
		currentCA := selfSignedPEM()
		rotatedCA := selfSignedPEM()
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			_, _ = w.Write(currentCA)
		}))
		defer srv.Close()
		serverPool := x509.NewCertPool()
		serverPool.AddCert(srv.Certificate())
		originalFetchClient := CAFetchClientFactory
		CAFetchClientFactory = func() *http.Client {
			return &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{RootCAs: serverPool},
				},
			}
		}
		defer func() { CAFetchClientFactory = originalFetchClient }()

		cfg := config.New()
		cfg.KubeConfig.SetForTest(kubeconfigWithCAURL(s.T(), srv.URL))
		cfg.CARefreshInterval.SetForTest(100 * time.Millisecond)
		manager, err := NewKubeconfigManager(s.T().Context(), cfg, "")
		s.Require().NoError(err)
		defer manager.Close()

		// The rest config must point at the cached file, not inline data,
		// so client-go's CA-file rotation applies.
		caFile := manager.kubernetes.RESTConfig().CAFile
		s.NotEmpty(caFile)
		s.Empty(manager.kubernetes.RESTConfig().CAData)
		got, err := os.ReadFile(caFile)
		s.Require().NoError(err)
		s.Equal(currentCA, got, "cached CA should match the initially served CA")

		// Rotate the served CA and wait for the refresher to pick it up.
		mu.Lock()
		currentCA = rotatedCA
		mu.Unlock()
		require.Eventually(s.T(), func() bool {
			got, err := os.ReadFile(caFile)
			return err == nil && string(got) == string(rotatedCA)
		}, 5*time.Second, 50*time.Millisecond)

		// Close is safe to call twice; the refresh goroutine must not leak.
		manager.Close()
		manager.Close()
	})
	s.Run("RefreshCAs picks up a rotated CA on demand (SIGHUP path)", func() {
		// Auto-refresh disabled; the refresher must still serve on-demand
		// refreshes via Manager.RefreshCAs so SIGHUP can unbreak a server
		// without waiting out an interval or restarting.
		var mu sync.Mutex
		currentCA := selfSignedPEM()
		rotatedCA := selfSignedPEM()
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			_, _ = w.Write(currentCA)
		}))
		defer srv.Close()
		serverPool := x509.NewCertPool()
		serverPool.AddCert(srv.Certificate())
		originalFetchClient := CAFetchClientFactory
		CAFetchClientFactory = func() *http.Client {
			return &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{RootCAs: serverPool},
				},
			}
		}
		defer func() { CAFetchClientFactory = originalFetchClient }()

		cfg := config.New()
		cfg.KubeConfig.SetForTest(kubeconfigWithCAURL(s.T(), srv.URL))
		cfg.CARefreshInterval.SetForTest(0) // no background re-fetch
		manager, err := NewKubeconfigManager(s.T().Context(), cfg, "")
		s.Require().NoError(err)
		defer manager.Close()
		caFile := manager.kubernetes.RESTConfig().CAFile
		s.Require().NotEmpty(caFile)

		// Rotate the served CA: without a refresher loop the cache file
		// must stay on the old CA until RefreshCAs is called.
		mu.Lock()
		currentCA = rotatedCA
		mu.Unlock()
		got, err := os.ReadFile(caFile)
		s.Require().NoError(err)
		s.NotEqual(rotatedCA, got, "cache must not change without an explicit refresh at interval 0")

		manager.RefreshCAs()
		got, err = os.ReadFile(caFile)
		s.Require().NoError(err)
		s.Equal(rotatedCA, got, "RefreshCAs must rewrite the cache with the rotated CA")
	})
	s.Run("rejects insecure-skip-tls-verify combined with a caURL", func() {
		// client-go would reject the CAFile later with a cryptic error
		// ("specifying a root certificates file with the insecure flag is
		// not allowed"); the manager must fail fast with a message naming
		// the conflict. Fails before any fetch, so no server is needed.
		kc := clientcmdapi.NewConfig()
		kc.Clusters["fake"] = &clientcmdapi.Cluster{
			Server:                "https://127.0.0.1:1",
			InsecureSkipTLSVerify: true,
			Extensions: map[string]apiruntime.Object{
				caExtensionName: extensionObject("https://ca.example.com/"),
			},
		}
		kc.Contexts["fake"] = &clientcmdapi.Context{Cluster: "fake", AuthInfo: "fake"}
		kc.AuthInfos["fake"] = &clientcmdapi.AuthInfo{Token: "test-token"}
		kc.CurrentContext = "fake"
		cfg := config.New()
		cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kc))
		cfg.CARefreshInterval.SetForTest(0)
		_, err := NewKubeconfigManager(s.T().Context(), cfg, "")
		s.Require().Error(err)
		s.ErrorContains(err, "insecure-skip-tls-verify")
	})
	s.Run("leaves rest config untouched without an extension", func() {
		path := s.mockServerKubeconfig()
		cfg := config.New()
		cfg.KubeConfig.SetForTest(path)
		cfg.CARefreshInterval.SetForTest(time.Minute)
		manager, err := NewKubeconfigManager(s.T().Context(), cfg, "")
		s.Require().NoError(err)
		defer manager.Close()
		s.Empty(manager.kubernetes.RESTConfig().CAFile)
	})
}

// caURLOfResolvedCluster loads the kubeconfig behind cfg and returns the caURL
// of its current context's cluster, exercising the real decode path.
func caURLOfResolvedCluster(t testing.TB, cfg *config.Config) string {
	t.Helper()
	raw, err := loadKubeconfigRaw(cfg.KubeConfig.Get())
	require.NoError(t, err)
	ctxInfo := raw.Contexts[raw.CurrentContext]
	require.NotNil(t, ctxInfo)
	return caURLFromClusterExtensions(raw.Clusters[ctxInfo.Cluster])
}

func loadKubeconfigRaw(path string) (*clientcmdapi.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return clientcmd.Load(data)
}

func (s *ClusterCACASuite) mockServerKubeconfig() string {
	// Reuse the mock server's kubeconfig shape but with a dummy endpoint;
	// the API server is never dialed during manager creation.
	raw, err := clientcmd.Load([]byte(`
apiVersion: v1
kind: Config
clusters:
- name: fake
  cluster:
    server: https://127.0.0.1:1
contexts:
- name: fake
  context:
    cluster: fake
    user: fake
current-context: fake
users:
- name: fake
  user:
    token: test-token
`))
	require.NoError(s.T(), err)
	return test.KubeconfigFile(s.T(), raw)
}
