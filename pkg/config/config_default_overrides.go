package config

// defaultOverrides is invoked from New after Option metadata is installed and
// before values are reset to Default. Downstream builds replace this file to
// change Option defaults and spellings (TOML / env names) plus
// ConfigPathEnvName.
func defaultOverrides(_ *Config) {
	// Intentionally left blank upstream.
}
