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
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ClusterCACASuite struct {
	suite.Suite
	originalEnv []string
}

func (s *ClusterCACASuite) SetupTest() {
	s.originalEnv = os.Environ()
}

func (s *ClusterCACASuite) TearDownTest() {
	test.RestoreEnv(s.originalEnv)
}

func TestClusterCASuite(t *testing.T) {
	suite.Run(t, new(ClusterCACASuite))
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
		Extensions: map[string]runtime.Object{
			caExtensionName: extensionObject(caURL),
		},
	}
	kc.Contexts["fake"] = &clientcmdapi.Context{Cluster: "fake", AuthInfo: "fake"}
	kc.AuthInfos["fake"] = &clientcmdapi.AuthInfo{Token: "test-token"}
	kc.CurrentContext = "fake"
	return test.KubeconfigFile(t, kc)
}

func extensionObject(caURL string) runtime.Object {
	raw, _ := json.Marshal(map[string]string{"caURL": caURL})
	return &runtime.Unknown{Raw: raw}
}

func (s *ClusterCACASuite) TestCAURLFromClusterExtensions() {
	s.Run("reads caURL from the kubernetes-mcp-server extension", func() {
		cluster := &clientcmdapi.Cluster{
			Extensions: map[string]runtime.Object{
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
			Extensions: map[string]runtime.Object{
				caExtensionName: &runtime.Unknown{Raw: []byte(`"just-a-string"`)},
			},
		}
		s.Empty(caURLFromClusterExtensions(cluster))
	})
	s.Run("survives a write/load round-trip of a real kubeconfig file", func() {
		// The kubeconfig path exercises the actual clientcmd decode of
		// extensions (runtime.Unknown payloads).
		path := kubeconfigWithCAURL(s.T(), "https://ca.example.com/")
		cfg := &config.StaticConfig{KubeConfig: path, CACacheDir: s.T().TempDir()}
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
		r.refreshOnce()

		info, err := os.Stat(cacheFile)
		s.Require().NoError(err)
		s.True(info.ModTime().Before(time.Now().Add(-30*time.Minute)), "an unchanged CA must not be rewritten")

		// No temp files left behind either.
		matches, _ := filepath.Glob(filepath.Join(cacheDir, ".ca-*.tmp"))
		s.Empty(matches)
	})
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

		cfg := &config.StaticConfig{
			KubeConfig:        kubeconfigWithCAURL(s.T(), srv.URL),
			CACacheDir:        filepath.Join(s.T().TempDir(), "ca-cache"),
			CARefreshInterval: config.Duration(100 * time.Millisecond),
		}
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
	s.Run("leaves rest config untouched without an extension", func() {
		path := s.mockServerKubeconfig()
		manager, err := NewKubeconfigManager(s.T().Context(), &config.StaticConfig{
			KubeConfig:        path,
			CACacheDir:        s.T().TempDir(),
			CARefreshInterval: config.Duration(time.Minute),
		}, "")
		s.Require().NoError(err)
		defer manager.Close()
		s.Empty(manager.kubernetes.RESTConfig().CAFile)
	})
}

// caURLOfResolvedCluster loads the kubeconfig behind cfg and returns the caURL
// of its current context's cluster, exercising the real decode path.
func caURLOfResolvedCluster(t testing.TB, cfg *config.StaticConfig) string {
	t.Helper()
	raw, err := loadKubeconfigRaw(cfg.GetKubeConfigPath())
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
