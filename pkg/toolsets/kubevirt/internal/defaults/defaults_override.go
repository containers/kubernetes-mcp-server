package defaults

const (
	toolsetDescriptionOverride               = ""
	productNameOverride                      = ""
	windowsEFIInstallerTektonCatalogOverride = ""
)

func ToolsetDescriptionOverride() string {
	return toolsetDescriptionOverride
}

func ProductNameOverride() string {
	return productNameOverride
}

func WindowsEFIInstallerTektonCatalogOverride() string {
	return windowsEFIInstallerTektonCatalogOverride
}
