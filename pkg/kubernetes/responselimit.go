package kubernetes

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// ErrBackendLimitUnavailable is returned when a call cannot read
// max_backend_response_bytes from the live config.
var ErrBackendLimitUnavailable = errors.New("max_backend_response_bytes is unavailable")

// BackendTruncationNotice is appended when a read that keeps a partial result
// hits max_backend_response_bytes.
const BackendTruncationNotice = "\n[truncated: max_backend_response_bytes]\n"

// BackendResponseTooLargeError is returned when a response body exceeds
// max_backend_response_bytes. The body is closed.
type BackendResponseTooLargeError struct {
	Limit int64
}

// AcceptTruncated returns body as text. A backend cap error becomes the bytes
// already read plus BackendTruncationNotice and a nil error, with truncated
// set. Any other error is returned with an empty string.
func AcceptTruncated(body []byte, err error) (text string, truncated bool, outErr error) {
	var tooLarge *BackendResponseTooLargeError
	if errors.As(err, &tooLarge) {
		return string(body) + BackendTruncationNotice, true, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(body), false, nil
}

func (e *BackendResponseTooLargeError) Error() string {
	return fmt.Sprintf("response exceeded max_backend_response_bytes (%d)", e.Limit)
}

// LimitResponseBody allows exactly limit bytes. The next byte closes body and
// returns *BackendResponseTooLargeError. limit <= 0 returns body unchanged.
func LimitResponseBody(body io.ReadCloser, limit int64) io.ReadCloser {
	if body == nil || limit <= 0 {
		return body
	}
	return &limitedBody{rc: body, limit: limit}
}

type limitedBody struct {
	rc     io.ReadCloser
	limit  int64
	read   int64
	err    error
	closed bool
	mu     sync.Mutex
}

func (b *limitedBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return 0, b.err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if b.read == b.limit {
		return b.peek()
	}
	room := b.limit - b.read
	if int64(len(p)) > room {
		p = p[:room]
	}
	n, err := b.rc.Read(p)
	b.read += int64(n)
	if b.read < b.limit || err != nil {
		return n, err
	}
	var one [1]byte
	extra, peekErr := b.rc.Read(one[:])
	if extra > 0 {
		return n, b.fail()
	}
	if peekErr != nil && peekErr != io.EOF {
		return n, peekErr
	}
	return n, nil
}

func (b *limitedBody) peek() (int, error) {
	var one [1]byte
	n, err := b.rc.Read(one[:])
	if n > 0 {
		return 0, b.fail()
	}
	if err == nil || err == io.EOF {
		return 0, io.EOF
	}
	return 0, err
}

func (b *limitedBody) fail() error {
	if b.err == nil {
		b.err = &BackendResponseTooLargeError{Limit: b.limit}
		b.closed = true
		_ = b.rc.Close()
	}
	return b.err
}

func (b *limitedBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	return b.rc.Close()
}

// backendResponseRoundTripper caps resp.Body using the live limit. Status 101
// and /openapi/ responses are left intact so upgrades and discovery stay unbounded.
type backendResponseRoundTripper struct {
	delegate http.RoundTripper
	limit    func() (int64, error)
}

func newBackendResponseRoundTripper(delegate http.RoundTripper, limit func() (int64, error)) http.RoundTripper {
	return &backendResponseRoundTripper{delegate: delegate, limit: limit}
}

// WrappedRoundTripper lets client-go unwrap this transport when closing idle connections.
func (rt *backendResponseRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return rt.delegate
}

func (rt *backendResponseRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := rt.delegate.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	if resp.StatusCode == http.StatusSwitchingProtocols {
		return resp, nil
	}
	if req != nil && req.URL != nil && strings.Contains(req.URL.Path, "/openapi/") {
		return resp, nil
	}
	// Unreachable today: every constructor passes a limit func. The error is
	// defense against a future bug that installs this wrapper without one.
	if rt.limit == nil {
		_ = resp.Body.Close()
		return nil, ErrBackendLimitUnavailable
	}
	limit, limitErr := rt.limit()
	if limitErr != nil {
		_ = resp.Body.Close()
		return nil, limitErr
	}
	resp.Body = LimitResponseBody(resp.Body, limit)
	return resp, nil
}
