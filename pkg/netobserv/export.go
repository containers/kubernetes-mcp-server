package netobserv

import "context"

// GetResponse is the result of a plugin HTTP GET call.
type GetResponse struct {
	Body string
}

// ExecuteGetAccept performs a GET with a custom Accept header.
// The body is capped by max_backend_response_bytes, the same limit as ExecuteGet.
func (n *NetObserv) ExecuteGetAccept(ctx context.Context, endpoint string, arguments map[string]any, accept string) (GetResponse, error) {
	return n.executeGet(ctx, endpoint, arguments, accept)
}
