package config

import (
	"fmt"
	"net/url"
)

// ValidateURLRequiresTLS validates that a URL uses HTTPS scheme when TLS is required.
// Returns nil if the URL is empty. Returns an error if the URL does not use HTTPS.
func ValidateURLRequiresTLS(urlStr string, fieldName string) error {
	if urlStr == "" {
		return nil
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", fieldName, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("require_tls is enabled but %s uses %q scheme (HTTPS required)", fieldName, u.Scheme)
	}
	return nil
}
