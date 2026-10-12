package kubernetes

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type SchemaCacheSuite struct {
	suite.Suite
}

func TestSchemaCache(t *testing.T) {
	suite.Run(t, new(SchemaCacheSuite))
}

func (s *SchemaCacheSuite) TestConcurrentValidatorsShareOneFetch() {
	doc := newOpenAPISchemaHandler()
	var entered atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	s.T().Cleanup(unblock)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v2" {
			http.NotFound(w, r)
			return
		}
		entered.Add(1)
		<-release
		hits.Add(1)
		doc.ServeHTTP(w, r)
	}))
	s.T().Cleanup(srv.Close)

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	s.Require().NoError(err)
	disc := cs.Discovery()
	cache := newOpenAPISchemaCache()
	req := podCreateRequest()

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var errs []error
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := newSchemaValidator(func() discovery.DiscoveryInterface { return disc }, cache)
			if err := v.Validate(s.T().Context(), req); err != nil {
				errMu.Lock()
				errs = append(errs, err)
				errMu.Unlock()
			}
		}()
	}

	s.Eventually(func() bool { return entered.Load() >= 1 }, time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	s.Equal(int32(1), entered.Load())
	unblock()
	wg.Wait()
	s.Empty(errs)
	s.Equal(int32(1), hits.Load())

	again := newSchemaValidator(func() discovery.DiscoveryInterface { return disc }, cache)
	s.NoError(again.Validate(s.T().Context(), req))
	s.Equal(int32(1), hits.Load())
}

func (s *SchemaCacheSuite) TestFailedFetchIsNotCached() {
	doc := newOpenAPISchemaHandler()
	var fail atomic.Bool
	fail.Store(true)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v2" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		if fail.Load() {
			http.Error(w, "unavailable", http.StatusInternalServerError)
			return
		}
		doc.ServeHTTP(w, r)
	}))
	s.T().Cleanup(srv.Close)

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	s.Require().NoError(err)
	v := newSchemaValidator(func() discovery.DiscoveryInterface { return cs.Discovery() }, newOpenAPISchemaCache())
	req := podCreateRequest()

	s.Error(v.Validate(s.T().Context(), req))
	fail.Store(false)
	s.NoError(v.Validate(s.T().Context(), req))
	s.Equal(int32(2), hits.Load())
}

func (s *SchemaCacheSuite) TestExpiredEntryIsFetchedAgain() {
	doc := newOpenAPISchemaHandler()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v2" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		doc.ServeHTTP(w, r)
	}))
	s.T().Cleanup(srv.Close)

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	s.Require().NoError(err)
	cache := newOpenAPISchemaCache()
	cache.ttl = -1
	v := newSchemaValidator(func() discovery.DiscoveryInterface { return cs.Discovery() }, cache)
	req := podCreateRequest()
	s.NoError(v.Validate(s.T().Context(), req))
	s.NoError(v.Validate(s.T().Context(), req))
	s.Equal(int32(2), hits.Load())
}

func podCreateRequest() *api.HTTPValidationRequest {
	return &api.HTTPValidationRequest{
		GVK:  &schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Verb: "create",
		Body: []byte(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test"},"spec":{"containers":[]}}`),
	}
}
