package kubernetes

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type BackendLimitSuite struct {
	suite.Suite
}

type chunkReader struct {
	data []byte
	size int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := c.size
	if n > len(p) {
		n = len(p)
	}
	if n > len(c.data) {
		n = len(c.data)
	}
	copy(p, c.data[:n])
	c.data = c.data[n:]
	return n, nil
}

type trackCloser struct {
	io.Reader
	closed bool
}

func (t *trackCloser) Close() error {
	t.closed = true
	return nil
}

func (s *BackendLimitSuite) readAll(body io.ReadCloser) ([]byte, error) {
	defer func() { _ = body.Close() }()
	return io.ReadAll(body)
}

func (s *BackendLimitSuite) TestBackendLimitRequiresConfig() {
	_, err := BackendLimit(nil)
	s.ErrorIs(err, ErrBackendLimitUnavailable)
	_, err = LiveBackendLimit(nil)()
	s.ErrorIs(err, ErrBackendLimitUnavailable)
}

func (s *BackendLimitSuite) TestReadCappedDropsOverCapBytes() {
	src := &trackCloser{Reader: strings.NewReader("abcdef")}
	data, err := ReadCapped(src, 4)
	s.Empty(data)
	s.True(IsResponseTooLarge(err))
	s.True(src.closed)

	data, err = ReadCapped(&trackCloser{Reader: strings.NewReader("abcd")}, 4)
	s.Require().NoError(err)
	s.Equal("abcd", string(data))
}

func (s *BackendLimitSuite) TestReadLimitedKeepsPrefix() {
	src := &trackCloser{Reader: strings.NewReader("abcdef")}
	data, err := ReadLimited(src, 4)
	s.Equal("abcd", string(data))
	s.True(IsResponseTooLarge(err))
	text, truncated, err := AcceptTruncated(data, err)
	s.Require().NoError(err)
	s.True(truncated)
	s.Equal("abcd"+BackendTruncationNotice, text)
}

func (s *BackendLimitSuite) TestUnderCap() {
	src := &trackCloser{Reader: strings.NewReader("abcd")}
	data, err := s.readAll(LimitResponseBody(src, 8))
	s.Require().NoError(err)
	s.Equal("abcd", string(data))
	s.True(src.closed)
}

func (s *BackendLimitSuite) TestExactCap() {
	src := &trackCloser{Reader: strings.NewReader("abcdefgh")}
	data, err := s.readAll(LimitResponseBody(src, 8))
	s.Require().NoError(err)
	s.Equal("abcdefgh", string(data))
	s.True(src.closed)
}

type readStep struct {
	data []byte
	err  error
}

type scriptedReader struct {
	steps []readStep
	i     int
	reads int
}

func (s *scriptedReader) Read(p []byte) (int, error) {
	s.reads++
	if s.i >= len(s.steps) {
		return 0, io.EOF
	}
	step := s.steps[s.i]
	s.i++
	return copy(p, step.data), step.err
}

func (s *BackendLimitSuite) TestZeroReadThenExtraByteIsTooLarge() {
	src := &trackCloser{Reader: &scriptedReader{steps: []readStep{
		{data: []byte("abcd")},
		{},
		{data: []byte("e")},
	}}}
	data, err := s.readAll(LimitResponseBody(src, 4))
	var tooLarge *BackendResponseTooLargeError
	s.Require().ErrorAs(err, &tooLarge)
	s.Equal(int64(4), tooLarge.Limit)
	s.Equal("abcd", string(data))
	s.True(src.closed)
}

func (s *BackendLimitSuite) TestZeroReadThenEOFIsExactFit() {
	src := &trackCloser{Reader: &scriptedReader{steps: []readStep{
		{data: []byte("abcd")},
		{},
		{err: io.EOF},
	}}}
	data, err := s.readAll(LimitResponseBody(src, 4))
	s.Require().NoError(err)
	s.Equal("abcd", string(data))
}

func (s *BackendLimitSuite) TestPersistentZeroReadFailsClosed() {
	src := &scriptedReader{}
	src.steps = []readStep{{data: []byte("abcd")}}
	// After the capped bytes, every read is (0, nil).
	body := LimitResponseBody(&trackCloser{Reader: &zeroReader{scriptedReader: src}}, 4)
	data, err := io.ReadAll(body)
	var tooLarge *BackendResponseTooLargeError
	s.Require().ErrorAs(err, &tooLarge)
	s.Equal("abcd", string(data))
	s.Equal(1+maxZeroReads, src.reads)
}

// zeroReader returns the scripted prefix, then (0, nil) forever.
type zeroReader struct {
	*scriptedReader
	done bool
}

func (z *zeroReader) Read(p []byte) (int, error) {
	if z.done {
		z.reads++
		return 0, nil
	}
	n, err := z.scriptedReader.Read(p)
	if z.i >= len(z.steps) {
		z.done = true
	}
	return n, err
}

func (s *BackendLimitSuite) TestPeekRetriesZeroReadThenFailsClosed() {
	src := &trackCloser{Reader: &peekThenByte{}}
	body := LimitResponseBody(src, 4)
	buf := make([]byte, 8)
	n, err := body.Read(buf)
	s.Equal(4, n)
	s.EqualError(err, "transient")
	n, err = body.Read(buf)
	var tooLarge *BackendResponseTooLargeError
	s.Require().ErrorAs(err, &tooLarge)
	s.Equal(0, n)
	s.True(src.closed)
}

// peekThenByte fills the cap with a transient error so the inline peek is
// skipped, then returns (0, nil) once and an extra byte.
type peekThenByte struct {
	reads int
}

func (p *peekThenByte) Read(buf []byte) (int, error) {
	p.reads++
	switch p.reads {
	case 1:
		return copy(buf, []byte("abcd")), errors.New("transient")
	case 2:
		return 0, nil
	default:
		if len(buf) == 0 {
			return 0, nil
		}
		buf[0] = 'e'
		return 1, nil
	}
}

func (s *BackendLimitSuite) TestChunkedOverCap() {
	src := &trackCloser{Reader: &chunkReader{data: []byte("abcdefghij"), size: 3}}
	body := LimitResponseBody(src, 5)
	data, err := io.ReadAll(body)
	var tooLarge *BackendResponseTooLargeError
	s.Require().ErrorAs(err, &tooLarge)
	s.Equal(int64(5), tooLarge.Limit)
	s.Equal("abcde", string(data))
	s.True(src.closed, "the overflowing read closes the body")
	s.NoError(body.Close())
}

func (s *BackendLimitSuite) TestDisabledCap() {
	src := &trackCloser{Reader: strings.NewReader("abcdefghij")}
	wrapped := LimitResponseBody(src, 0)
	s.Same(src, wrapped)
	data, err := s.readAll(wrapped)
	s.Require().NoError(err)
	s.Equal("abcdefghij", string(data))
}

func (s *BackendLimitSuite) TestRoundTripSkipsUpgradeAndMarkedCalls() {
	const limit = int64(4)
	payload := []byte("0123456789")
	delegate := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if req.URL.Path == "/upgrade" {
			status = http.StatusSwitchingProtocols
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewReader(payload)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	rt := newBackendResponseRoundTripper(delegate, func() (int64, error) { return limit, nil })

	s.Run("101 is not capped", func() {
		req := httptest.NewRequest(http.MethodGet, "https://apiserver/upgrade", nil)
		resp, err := rt.RoundTrip(req)
		s.Require().NoError(err)
		data, err := s.readAll(resp.Body)
		s.Require().NoError(err)
		s.Equal(payload, data)
	})
	s.Run("/openapi/v2 is capped", func() {
		req := httptest.NewRequest(http.MethodGet, "https://apiserver/openapi/v2", nil)
		resp, err := rt.RoundTrip(req)
		s.Require().NoError(err)
		data, err := io.ReadAll(resp.Body)
		var tooLarge *BackendResponseTooLargeError
		s.Require().ErrorAs(err, &tooLarge)
		s.Equal(payload[:limit], data)
	})
	s.Run("a marked request is not capped", func() {
		req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", nil)
		req = req.WithContext(SkipBackendResponseLimit(req.Context()))
		resp, err := rt.RoundTrip(req)
		s.Require().NoError(err)
		data, err := s.readAll(resp.Body)
		s.Require().NoError(err)
		s.Equal(payload, data)
	})
	s.Run("namespace named openapi is capped", func() {
		req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/namespaces/openapi/pods", nil)
		resp, err := rt.RoundTrip(req)
		s.Require().NoError(err)
		data, err := io.ReadAll(resp.Body)
		var tooLarge *BackendResponseTooLargeError
		s.Require().ErrorAs(err, &tooLarge)
		s.Equal(payload[:limit], data)
	})
	s.Run("a normal response is capped", func() {
		req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", nil)
		resp, err := rt.RoundTrip(req)
		s.Require().NoError(err)
		data, err := io.ReadAll(resp.Body)
		var tooLarge *BackendResponseTooLargeError
		s.Require().ErrorAs(err, &tooLarge)
		s.Equal(payload[:limit], data)
	})
	s.Run("limit 0 leaves the body uncapped", func() {
		open := newBackendResponseRoundTripper(delegate, func() (int64, error) { return 0, nil })
		req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", nil)
		resp, err := open.RoundTrip(req)
		s.Require().NoError(err)
		data, err := s.readAll(resp.Body)
		s.Require().NoError(err)
		s.Equal(payload, data)
	})
}

func (s *BackendLimitSuite) TestClientGoStopsBeforeDecode() {
	const limit = int64(8)
	body := []byte(`{"kind":"Pod","apiVersion":"v1","metadata":{"name":"p"}}`)
	s.Greater(len(body), int(limit))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	s.T().Cleanup(srv.Close)

	cfg := &rest.Config{Host: srv.URL}
	cfg.GroupVersion = &v1.SchemeGroupVersion
	cfg.APIPath = "/api"
	cfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return newBackendResponseRoundTripper(rt, func() (int64, error) { return limit, nil })
	})
	client, err := rest.RESTClientFor(cfg)
	s.Require().NoError(err)

	var pod v1.Pod
	err = client.Get().AbsPath("/api/v1/pods/p").Do(s.T().Context()).Into(&pod)
	var tooLarge *BackendResponseTooLargeError
	s.Require().ErrorAs(err, &tooLarge)
	s.Equal(limit, tooLarge.Limit)
	s.Empty(pod.Name)
}

func (s *BackendLimitSuite) TestRoundTripReadsTheLimitEachCall() {
	var limit int64 = 100
	delegate := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("0123456789")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	rt := newBackendResponseRoundTripper(delegate, func() (int64, error) { return limit, nil })

	req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", nil)
	resp, err := rt.RoundTrip(req)
	s.Require().NoError(err)
	data, err := s.readAll(resp.Body)
	s.Require().NoError(err)
	s.Equal("0123456789", string(data))

	limit = 4
	resp, err = rt.RoundTrip(req)
	s.Require().NoError(err)
	data, err = io.ReadAll(resp.Body)
	var tooLarge *BackendResponseTooLargeError
	s.Require().ErrorAs(err, &tooLarge)
	s.Equal("0123", string(data))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func (s *BackendLimitSuite) TestAcceptTruncatedKeepsThePrefix() {
	notice := BackendTruncationNotice
	body := []byte("abcd")
	text, truncated, err := AcceptTruncated(body, &BackendResponseTooLargeError{Limit: 4})
	s.Require().NoError(err)
	s.True(truncated)
	s.Equal("abcd"+notice, text)

	text, truncated, err = AcceptTruncated(body, nil)
	s.Require().NoError(err)
	s.False(truncated)
	s.Equal("abcd", text)

	text, truncated, err = AcceptTruncated(body, errors.New("reset"))
	s.ErrorContains(err, "reset")
	s.False(truncated)
	s.Empty(text)
}

func (s *BackendLimitSuite) TestRoundTripErrorsWhenTheLimitIsUnavailable() {
	src := &trackCloser{Reader: strings.NewReader("abcdefghij")}
	delegate := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       src,
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	rt := newBackendResponseRoundTripper(delegate, func() (int64, error) {
		return 0, ErrBackendLimitUnavailable
	})
	req := httptest.NewRequest(http.MethodGet, "https://apiserver/api/v1/pods", nil)
	resp, err := rt.RoundTrip(req)
	s.Nil(resp)
	s.ErrorIs(err, ErrBackendLimitUnavailable)
	s.True(src.closed)
}

func TestBackendLimit(t *testing.T) {
	suite.Run(t, new(BackendLimitSuite))
}
