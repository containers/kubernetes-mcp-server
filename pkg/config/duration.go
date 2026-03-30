package config

import (
	"fmt"
	"time"
)

// Duration is a time.Duration that can be unmarshaled from TOML.
// It accepts Go duration strings like "30s", "5m", "1h30m".
// Note: Negative durations are accepted but effectively disable timeouts
// when used with http.Server (which treats zero/negative as no timeout).
type Duration time.Duration

// Duration returns the underlying time.Duration value.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// UnmarshalText implements encoding.TextUnmarshaler for TOML parsing.
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", text, err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalText implements encoding.TextMarshaler for TOML serialization.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}
