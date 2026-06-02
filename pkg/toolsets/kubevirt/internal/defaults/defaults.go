package defaults

const (
	DefaultToolsetDescription               = "KubeVirt virtual machine management tools, check the [KubeVirt documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/kubevirt.md) for more details."
	DefaultProductName                      = "KubeVirt"
	DefaultWindowsEFIInstallerTektonCatalog = "kubevirt-tekton-pipelines"
)

func ToolsetDescription() string {
	overrideDescription := ToolsetDescriptionOverride()
	if overrideDescription != "" {
		return overrideDescription
	}
	return DefaultToolsetDescription
}

func ProductName() string {
	overrideProductName := ProductNameOverride()
	if overrideProductName != "" {
		return overrideProductName
	}
	return DefaultProductName
}

func WindowsEFIInstallerTektonCatalog() string {
	overrideCatalog := WindowsEFIInstallerTektonCatalogOverride()
	if overrideCatalog != "" {
		return overrideCatalog
	}
	return DefaultWindowsEFIInstallerTektonCatalog
}
