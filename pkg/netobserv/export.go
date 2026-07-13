package netobserv

import "context"

// GetResponse is the result of a plugin HTTP GET call.
type GetResponse struct {
	Body      string
	Truncated bool
}

// ExecuteGetAccept performs a GET with a custom Accept header and truncates the body at maxBodySize.
func (n *NetObserv) ExecuteGetAccept(ctx context.Context, endpoint string, arguments map[string]any, accept string, maxBodySize int64) (GetResponse, error) {
	return n.executeGet(ctx, endpoint, arguments, accept, maxBodySize, true)
}
