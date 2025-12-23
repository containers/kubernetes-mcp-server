package test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

type MockServer struct {
	server       *httptest.Server
	config       *rest.Config
	restHandlers []http.HandlerFunc
}

func NewMockServer() *MockServer {
	ms := &MockServer{}
	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)
	ms.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		for _, handler := range ms.restHandlers {
			handler(w, req)
		}
	}))
	ms.config = &rest.Config{
		Host:    ms.server.URL,
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			NegotiatedSerializer: codecs,
			ContentType:          runtime.ContentTypeJSON,
			GroupVersion:         &v1.SchemeGroupVersion,
		},
	}
	ms.restHandlers = make([]http.HandlerFunc, 0)
	return ms
}

func (m *MockServer) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

func (m *MockServer) Handle(handler http.Handler) {
	m.restHandlers = append(m.restHandlers, handler.ServeHTTP)
}

func (m *MockServer) ResetHandlers() {
	m.restHandlers = make([]http.HandlerFunc, 0)
}

func (m *MockServer) Config() *rest.Config {
	return m.config
}

func (m *MockServer) Kubeconfig() *api.Config {
	fakeConfig := KubeConfigFake()
	fakeConfig.Clusters["fake"].Server = m.config.Host
	fakeConfig.Clusters["fake"].CertificateAuthorityData = m.config.CAData
	fakeConfig.AuthInfos["fake"].ClientKeyData = m.config.KeyData
	fakeConfig.AuthInfos["fake"].ClientCertificateData = m.config.CertData
	return fakeConfig
}

func (m *MockServer) KubeconfigFile(t *testing.T) string {
	return KubeconfigFile(t, m.Kubeconfig())
}

func KubeconfigFile(t *testing.T, kubeconfig *api.Config) string {
	kubeconfigFile := filepath.Join(t.TempDir(), "config")
	err := clientcmd.WriteToFile(*kubeconfig, kubeconfigFile)
	require.NoError(t, err, "Expected no error writing kubeconfig file")
	return kubeconfigFile
}

func WriteObject(w http.ResponseWriter, obj runtime.Object) {
	w.Header().Set("Content-Type", runtime.ContentTypeJSON)
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type streamAndReply struct {
	httpstream.Stream
	replySent <-chan struct{}
}

type StreamContext struct {
	Closer       io.Closer
	StdinStream  io.ReadCloser
	StdoutStream io.WriteCloser
	StderrStream io.WriteCloser
	writeStatus  func(status *apierrors.StatusError) error
}

type StreamOptions struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func v4WriteStatusFunc(stream io.Writer) func(status *apierrors.StatusError) error {
	return func(status *apierrors.StatusError) error {
		bs, err := json.Marshal(status.Status())
		if err != nil {
			return err
		}
		_, err = stream.Write(bs)
		return err
	}
}
func CreateHTTPStreams(w http.ResponseWriter, req *http.Request, opts *StreamOptions) (*StreamContext, error) {
	_, err := httpstream.Handshake(req, w, []string{"v4.channel.k8s.io"})
	if err != nil {
		return nil, err
	}

	upgrader := spdy.NewResponseUpgrader()
	streamCh := make(chan streamAndReply)
	connection := upgrader.UpgradeResponse(w, req, func(stream httpstream.Stream, replySent <-chan struct{}) error {
		streamCh <- streamAndReply{Stream: stream, replySent: replySent}
		return nil
	})
	ctx := &StreamContext{
		Closer: connection,
	}

	// wait for stream
	replyChan := make(chan struct{}, 4)
	defer close(replyChan)
	receivedStreams := 0
	expectedStreams := 1
	if opts.Stdout != nil {
		expectedStreams++
	}
	if opts.Stdin != nil {
		expectedStreams++
	}
	if opts.Stderr != nil {
		expectedStreams++
	}
WaitForStreams:
	for {
		select {
		case stream := <-streamCh:
			streamType := stream.Headers().Get(v1.StreamType)
			switch streamType {
			case v1.StreamTypeError:
				replyChan <- struct{}{}
				ctx.writeStatus = v4WriteStatusFunc(stream)
			case v1.StreamTypeStdout:
				replyChan <- struct{}{}
				ctx.StdoutStream = stream
			case v1.StreamTypeStdin:
				replyChan <- struct{}{}
				ctx.StdinStream = stream
			case v1.StreamTypeStderr:
				replyChan <- struct{}{}
				ctx.StderrStream = stream
			default:
				// add other stream ...
				return nil, errors.New("unimplemented stream type")
			}
		case <-replyChan:
			receivedStreams++
			if receivedStreams == expectedStreams {
				break WaitForStreams
			}
		}
	}

	return ctx, nil
}

type DiscoveryClientHandler struct {
	// APIResourceLists defines all API groups and their resources.
	// The handler automatically generates /api, /apis, and /apis/<group>/<version> endpoints.
	APIResourceLists []metav1.APIResourceList
}

var _ http.Handler = (*DiscoveryClientHandler)(nil)

// NewDiscoveryClientHandler creates a DiscoveryClientHandler with default Kubernetes resources.
func NewDiscoveryClientHandler(additionalResources ...metav1.APIResourceList) *DiscoveryClientHandler {
	handler := &DiscoveryClientHandler{
		APIResourceLists: []metav1.APIResourceList{
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "nodes", Kind: "Node", Namespaced: false, Verbs: metav1.Verbs{"get", "list", "watch"}},
					{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "watch", "create", "update", "patch", "delete"}},
				},
			},
			{
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{
					{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "watch", "create", "update", "patch", "delete"}},
				},
			},
		},
	}
	handler.APIResourceLists = append(handler.APIResourceLists, additionalResources...)
	return handler
}

func (h *DiscoveryClientHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Request Performed by DiscoveryClient to Kube API (Get API Groups legacy -core-)
	if req.URL.Path == "/api" {
		WriteObject(w, &metav1.APIVersions{
			Versions:                   []string{"v1"},
			ServerAddressByClientCIDRs: []metav1.ServerAddressByClientCIDR{{ClientCIDR: "0.0.0.0/0"}},
		})
		return
	}

	// Request Performed by DiscoveryClient to Kube API (Get API Groups)
	if req.URL.Path == "/apis" {
		groups := make([]metav1.APIGroup, 0)
		for _, rl := range h.APIResourceLists {
			if rl.GroupVersion == "v1" {
				continue // Skip core API group, it's exposed via /api
			}
			group, version := parseGroupVersion(rl.GroupVersion)
			groups = append(groups, metav1.APIGroup{
				Name: group,
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: rl.GroupVersion, Version: version},
				},
				PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: rl.GroupVersion, Version: version},
			})
		}
		WriteObject(w, &metav1.APIGroupList{Groups: groups})
		return
	}

	// Request Performed by DiscoveryClient to Kube API (Get API Resources for core v1)
	if req.URL.Path == "/api/v1" {
		for _, rl := range h.APIResourceLists {
			if rl.GroupVersion == "v1" {
				WriteObject(w, &rl)
				return
			}
		}
		return
	}

	// Request Performed by DiscoveryClient to Kube API (Get API Resources for a group/version)
	if strings.HasPrefix(req.URL.Path, "/apis/") {
		pathParts := strings.Split(strings.TrimPrefix(req.URL.Path, "/apis/"), "/")
		if len(pathParts) == 2 {
			requestedGV := pathParts[0] + "/" + pathParts[1]
			for _, rl := range h.APIResourceLists {
				if rl.GroupVersion == requestedGV {
					WriteObject(w, &rl)
					return
				}
			}
		}
	}
}

// parseGroupVersion splits a groupVersion string (e.g., "apps/v1") into group and version.
func parseGroupVersion(gv string) (group, version string) {
	parts := strings.Split(gv, "/")
	if len(parts) == 1 {
		return "", parts[0] // Core API (e.g., "v1")
	}
	return parts[0], parts[1]
}

// AddAPIResourceList adds an API resource list to the handler.
// This is useful for dynamically modifying the handler during tests.
func (h *DiscoveryClientHandler) AddAPIResourceList(resourceList metav1.APIResourceList) {
	h.APIResourceLists = append(h.APIResourceLists, resourceList)
}

// NewInOpenShiftHandler creates a DiscoveryClientHandler configured for OpenShift clusters.
// It includes the OpenShift project.openshift.io API group by default.
// Additional API resource lists can be passed to extend the handler.
func NewInOpenShiftHandler(additionalResources ...metav1.APIResourceList) *DiscoveryClientHandler {
	openShiftResources := []metav1.APIResourceList{
		{
			GroupVersion: "project.openshift.io/v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "projects",
					Kind:       "Project",
					Namespaced: false,
					ShortNames: []string{"pr"},
					Verbs:      metav1.Verbs{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		},
	}
	openShiftResources = append(openShiftResources, additionalResources...)
	return NewDiscoveryClientHandler(openShiftResources...)
}
