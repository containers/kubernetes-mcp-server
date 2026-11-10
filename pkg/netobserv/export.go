package netobserv

import "context"

// GetResponse is the result of a plugin HTTP GET call.
type GetResponse struct {
	Body string
}

// ExecuteGetAccept performs a GET with a custom Accept header.
// The body is capped by max_backend_response_bytes. Past that cap the prefix
// is kept and a truncation notice is appended. ExecuteGet still returns an error.
func (n *NetObserv) ExecuteGetAccept(ctx context.Context, endpoint string, arguments map[string]any, accept string) (GetResponse, error) {
	return n.executeGet(ctx, endpoint, arguments, accept, true)
}
