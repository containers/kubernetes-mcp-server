package kcp

import (
	"bytes"
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
	"sync"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type KcpProviderSuite struct {
	suite.Suite
}

func TestKcpProviderSuite(t *testing.T) {
	suite.Run(t, new(KcpProviderSuite))
}

// kcpSelfSignedPEM returns a fresh self-signed CA certificate in PEM form.
func kcpSelfSignedPEM() []byte {
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "kcp-test-ca"},
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

func (s *KcpProviderSuite) TestResetKeepsDefaultWorkspaceCARefresherAlive() {
	// A reset runs during provider construction. It must not close the
	// freshly built base manager: that would kill the background CA
	// refresher and the default workspace would never pick up a rotated
	// cluster CA. The cache file following the served CA is the observable
	// proof the refresher survived.
	caA := kcpSelfSignedPEM()
	caB := kcpSelfSignedPEM()
	var mu sync.Mutex
	currentCA := caA
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write(currentCA)
	}))
	defer srv.Close()

	// The caURL fetch client must trust the test TLS server; pin its cert
	// via the injectable factory without touching the system cert pool.
	serverPool := x509.NewCertPool()
	serverPool.AddCert(srv.Certificate())
	originalFactory := kubernetes.CAFetchClientFactory
	kubernetes.CAFetchClientFactory = func() *http.Client {
		return &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: serverPool}},
		}
	}
	defer func() { kubernetes.CAFetchClientFactory = originalFactory }()

	raw, _ := json.Marshal(map[string]string{"caURL": srv.URL})
	kc := clientcmdapi.NewConfig()
	kc.Clusters["fake"] = &clientcmdapi.Cluster{
		// Never dialed: the API server port is closed, so workspace
		// discovery fails fast and falls back to the kubeconfig entries.
		Server: "https://127.0.0.1:1/clusters/root",
		Extensions: map[string]runtime.Object{
			"kubernetes-mcp-server": &runtime.Unknown{Raw: raw},
		},
	}
	kc.Contexts["fake"] = &clientcmdapi.Context{Cluster: "fake", AuthInfo: "fake"}
	kc.AuthInfos["fake"] = &clientcmdapi.AuthInfo{Token: "test-token"}
	kc.CurrentContext = "fake"
	kubeconfigPath := test.KubeconfigFile(s.T(), kc)

	cfg := config.New()
	cfg.KubeConfig.SetForTest(kubeconfigPath)
	cfg.CARefreshInterval.SetForTest(100 * time.Millisecond)
	provider, err := newKcpClusterProvider(s.T().Context(), cfg)
	s.Require().NoError(err)
	defer provider.Close()

	p := provider.(*kcpClusterProvider)
	baseManager := p.managers[p.defaultWorkspace]
	s.Require().NotNil(baseManager, "base manager should back the default workspace")
	caFile := baseManager.RESTConfig().CAFile
	s.Require().NotEmpty(caFile, "base manager should resolve the caURL extension to a cached CA file")

	got, err := os.ReadFile(caFile)
	s.Require().NoError(err)
	s.Equal(caA, got, "cached CA should match the CA served at construction")

	// Rotate the served CA; only a live refresher (one reset did not close)
	// converges the cache file.
	mu.Lock()
	currentCA = caB
	mu.Unlock()
	require.Eventually(s.T(), func() bool {
		got, err := os.ReadFile(caFile)
		return err == nil && bytes.Equal(got, caB)
	}, 5*time.Second, 50*time.Millisecond, "the cache file should follow the served CA rotation")
}

// kcpTestKubeconfig returns a kubeconfig path whose current cluster is the
// kcp base URL. The API server port is closed, so workspace discovery fails
// fast and falls back to the kubeconfig entries; nothing is dialed.
func kcpTestKubeconfig(t *testing.T) string {
	t.Helper()
	kc := clientcmdapi.NewConfig()
	kc.Clusters["fake"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:1/clusters/root"}
	kc.Contexts["fake"] = &clientcmdapi.Context{Cluster: "fake", AuthInfo: "fake"}
	kc.AuthInfos["fake"] = &clientcmdapi.AuthInfo{Token: "test-token"}
	kc.CurrentContext = "fake"
	return test.KubeconfigFile(t, kc)
}

// TestCloseRacesReset exercises Close concurrently with reset(), which
// replaces p.managers while holding p.mu. Close snapshots the managers under
// p.mu and closes them after releasing it: it must not deadlock against
// reset's p.mu -> WatcherGroup lock order, and repeated Close calls must stay
// safe.
func (s *KcpProviderSuite) TestCloseRacesReset() {
	for i := 0; i < 5; i++ {
		cfg := config.New()
		cfg.KubeConfig.SetForTest(kcpTestKubeconfig(s.T()))
		provider, err := newKcpClusterProvider(s.T().Context(), cfg)
		s.Require().NoError(err)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A reset that loses the race returns errProviderClosed.
			_ = provider.(*kcpClusterProvider).reset(s.T().Context())
		}()
		// Several closers run the shutdown path concurrently with the reset
		// that is installing replacement managers.
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				provider.Close()
			}()
		}
		wg.Wait()
		// Close stays idempotent after the race.
		provider.Close()
	}
}
