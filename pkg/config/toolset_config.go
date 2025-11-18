package config

var toolsetConfigRegistry = NewExtendedConfigRegistry()

func RegisterToolsetConfig(name string, parser ExtendedConfigParser) {
	toolsetConfigRegistry.Register(name, parser)
}
