package mcp

import (
	"encoding/json"
	"errors"
	"io"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/rest"
	"net/http"
	"net/http/httptest"
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
	m.server.Close()
}

func (m *MockServer) Handle(handler http.Handler) {
	m.restHandlers = append(m.restHandlers, handler.ServeHTTP)
}

func writeObject(w http.ResponseWriter, obj runtime.Object) {
	w.Header().Set("Content-Type", runtime.ContentTypeJSON)
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type streamAndReply struct {
	httpstream.Stream
	replySent <-chan struct{}
}

type streamContext struct {
	conn         io.Closer
	stdinStream  io.ReadCloser
	stdoutStream io.WriteCloser
	stderrStream io.WriteCloser
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
func createHTTPStreams(w http.ResponseWriter, req *http.Request, opts *StreamOptions) (*streamContext, error) {
	_, err := httpstream.Handshake(req, w, []string{"v4.channel.k8s.io"})
	if err != nil {
		return nil, err
	}

	upgrader := spdy.NewResponseUpgrader()
	streamCh := make(chan streamAndReply)
	conn := upgrader.UpgradeResponse(w, req, func(stream httpstream.Stream, replySent <-chan struct{}) error {
		streamCh <- streamAndReply{Stream: stream, replySent: replySent}
		return nil
	})
	ctx := &streamContext{
		conn: conn,
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
				ctx.stdoutStream = stream
			case v1.StreamTypeStdin:
				replyChan <- struct{}{}
				ctx.stdinStream = stream
			case v1.StreamTypeStderr:
				replyChan <- struct{}{}
				ctx.stderrStream = stream
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
