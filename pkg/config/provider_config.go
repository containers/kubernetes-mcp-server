package config

var providerConfigRegistry = NewExtendedConfigRegistry()

func RegisterProviderConfig(name string, parser ExtendedConfigParser) {
	providerConfigRegistry.Register(name, parser)
}
