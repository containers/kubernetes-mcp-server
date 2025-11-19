package config

var providerConfigRegistry = newExtendedConfigRegistry()

func RegisterProviderConfig(name string, parser ExtendedConfigParser) {
	providerConfigRegistry.register(name, parser)
}
