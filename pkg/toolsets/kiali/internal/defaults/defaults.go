package defaults

const (
	DefaultToolsetName        = "kiali"
	DefaultToolsetDescription = "Most common tools for managing Kiali, check the [Kiali documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/KIALI.md) for more details."
)

func ToolsetName() string {
	overrideName := ToolsetNameOverride()
	if overrideName != "" {
		return overrideName
	}
	return DefaultToolsetName
}

func ToolsetDescription() string {
	overrideDescription := ToolsetDescriptionOverride()
	if overrideDescription != "" {
		return overrideDescription
	}
	return DefaultToolsetDescription
}
