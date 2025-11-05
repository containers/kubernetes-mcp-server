package kiali

import (
	"context"
	"errors"
	"net/url"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// Config holds Kiali toolset configuration
type Config struct {
	Url      string `toml:"url,omitempty"`
	Insecure bool   `toml:"insecure,omitempty"`
}

var _ config.ToolsetConfig = (*Config)(nil)

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("kiali config is nil")
	}
	if c.Url == "" {
		return errors.New("kiali-url is required")
	}
	if u, err := url.Parse(c.Url); err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("kiali-url must be a valid URL")
	}
	return nil
}

func kialiToolsetParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (config.ToolsetConfig, error) {
	var cfg Config
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func init() {
	config.RegisterToolsetConfig("kiali", kialiToolsetParser)
}
