package netobserv

import "context"

// ExecuteGetAccept is like ExecuteGet but sets a custom Accept header (e.g. for CSV export).
func (n *NetObserv) ExecuteGetAccept(ctx context.Context, endpoint string, arguments map[string]any, accept string) (string, error) {
	return n.executeGet(ctx, endpoint, arguments, accept)
}
