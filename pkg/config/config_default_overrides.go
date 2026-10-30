package config

// defaultOverrides is invoked from New after Option metadata is installed and
// before values are reset to Default. Downstream builds replace this file to
// change Option defaults and spellings (TOML / env names) plus
// ConfigPathEnvName.
//
// Any Option default may change here, so tests must not assume the defaults
// recorded in this file: a test whose outcome depends on an Option must set
// it explicitly (config file, flag, or SetForTest) so it starts from the
// conditions its scenario requires.
func defaultOverrides(_ *Config) {
	// Intentionally left blank upstream.
}
