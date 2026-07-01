package defaults

const (
	DefaultToolsetName        = "netobserv"
	DefaultToolsetDescription = "Network observability tools backed by the NetObserv console plugin API (flows, metrics, export). Check the [NetObserv documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/NETOBSERV.md) for more details."
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
