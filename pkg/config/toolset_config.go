package config

var toolsetConfigRegistry = newExtendedConfigRegistry()

func RegisterToolsetConfig(name string, parser ExtendedConfigParser) {
	toolsetConfigRegistry.register(name, parser)
}
