package kubernetes

import "net/http"

type UserAgentRoundTripper struct {
	delegate http.RoundTripper
}

var _ http.RoundTripper = &UserAgentRoundTripper{}

func (u *UserAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	userAgent := req.Context().Value(UserAgentHeader)

	if userAgent == nil {
		return u.delegate.RoundTrip(req)
	}

	userAgentHeader, ok := userAgent.(string)
	if !ok || userAgentHeader == "" {
		return u.delegate.RoundTrip(req)
	}

	req = req.Clone(req.Context())

	req.Header.Set(string(UserAgentHeader), userAgentHeader)
	return u.delegate.RoundTrip(req)
}
