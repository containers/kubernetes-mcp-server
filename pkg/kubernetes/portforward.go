package kubernetes

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// portForwardReadyTimeout is how long we wait for the forwarder to signal
// readiness before returning to the caller.
const portForwardReadyTimeout = 5 * time.Second

// PortForwardSession represents an active port-forward session.
type PortForwardSession struct {
	ID         string
	Namespace  string
	PodName    string
	LocalPort  int
	RemotePort int
	stopChan   chan struct{}
	readyChan  chan struct{}
	out        *stringsBuilder
	forwarder  *portforward.PortForwarder
	cancel     context.CancelFunc
	done       chan struct{}
}

var (
	portForwardMu       sync.Mutex
	portForwardSessions = make(map[string]*PortForwardSession)
	portForwardCounter  atomic.Uint64
)

// stringsBuilder is a minimal io.Writer wrapping a byte buffer with a mutex.
type stringsBuilder struct {
	b []byte
	m sync.Mutex
}

func (sb *stringsBuilder) Write(p []byte) (int, error) {
	sb.m.Lock()
	defer sb.m.Unlock()
	sb.b = append(sb.b, p...)
	return len(p), nil
}

func (sb *stringsBuilder) String() string {
	sb.m.Lock()
	defer sb.m.Unlock()
	return string(sb.b)
}

// PodsPortForward starts a port-forward from a local port to a port on a pod.
// It runs in the background and returns a session id that can be used to stop
// the forward later. If localPort is 0, a free port is chosen.
func (c *Core) PodsPortForward(ctx context.Context, namespace, name string, localPort, remotePort int) (*PortForwardSession, error) {
	namespace = c.NamespaceOrDefault(namespace)
	pods := c.CoreV1().Pods(namespace)
	pod, err := pods.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, name, err)
	}
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		return nil, fmt.Errorf("cannot port-forward to a completed pod; current phase is %s", pod.Status.Phase)
	}

	// Choose a free local port if none requested.
	if localPort == 0 {
		free, ferr := freePort()
		if ferr != nil {
			return nil, fmt.Errorf("failed to find a free local port: %w", ferr)
		}
		localPort = free
	}

	req := c.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(name).
		SubResource("portforward")

	restConfig, err := c.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get REST config: %w", err)
	}
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create SPDY round tripper: %w", err)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{}, 1)
	sb := &stringsBuilder{}

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	fw, err := portforward.New(dialer, ports, stopChan, readyChan, sb, sb)
	if err != nil {
		return nil, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	session := &PortForwardSession{
		ID:         fmt.Sprintf("pf-%d", portForwardCounter.Add(1)),
		Namespace:  namespace,
		PodName:    name,
		LocalPort:  localPort,
		RemotePort: remotePort,
		stopChan:   stopChan,
		readyChan:  readyChan,
		out:        sb,
		forwarder:  fw,
		cancel:     cancel,
		done:       done,
	}

	portForwardMu.Lock()
	portForwardSessions[session.ID] = session
	portForwardMu.Unlock()

	go func() {
		defer close(done)
		defer cancel()
		_ = fw.ForwardPorts()
	}()

	// Wait for readiness, timeout, or early exit.
	select {
	case <-readyChan:
	case <-time.After(portForwardReadyTimeout):
		// The forwarder did not signal readiness; it may still be dialing.
	case <-done:
		return session, fmt.Errorf("port forwarder exited: %s", sb.String())
	}

	return session, nil
}

// StopPortForward stops a port-forward session by id.
func StopPortForward(id string) error {
	portForwardMu.Lock()
	session, ok := portForwardSessions[id]
	portForwardMu.Unlock()
	if !ok {
		return fmt.Errorf("port forward session %s not found", id)
	}
	close(session.stopChan)
	session.cancel()
	<-session.done
	portForwardMu.Lock()
	delete(portForwardSessions, id)
	portForwardMu.Unlock()
	return nil
}

// ListPortForwards returns a summary of active port-forward sessions.
func ListPortForwards() []PortForwardSummary {
	portForwardMu.Lock()
	defer portForwardMu.Unlock()
	out := make([]PortForwardSummary, 0, len(portForwardSessions))
	for _, s := range portForwardSessions {
		out = append(out, PortForwardSummary{
			ID:         s.ID,
			Namespace:  s.Namespace,
			PodName:    s.PodName,
			LocalPort:  s.LocalPort,
			RemotePort: s.RemotePort,
		})
	}
	return out
}

// PortForwardSummary is a serializable view of a port-forward session.
type PortForwardSummary struct {
	ID         string
	Namespace  string
	PodName    string
	LocalPort  int
	RemotePort int
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr := l.Addr().(*net.TCPAddr)
	return addr.Port, nil
}
