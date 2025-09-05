package kubernetes

import "net/http"

// nolint:unused
type impersonateRoundTripper struct {
	delegate http.RoundTripper
}

// nolint:unused
func (irt *impersonateRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// TODO: Solution won't work with discoveryclient which uses context.TODO() instead of the passed-in context
	if v, ok := req.Context().Value(OAuthAuthorizationHeader).(string); ok {
		req.Header.Set("Authorization", v)
	}
	return irt.delegate.RoundTrip(req)
}
