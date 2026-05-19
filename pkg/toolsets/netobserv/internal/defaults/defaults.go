package defaults

const (
	DefaultToolsetName        = "netobserv"
	DefaultToolsetDescription = "Network observability tools backed by the NetObserv console plugin API (flows, metrics, alerts, export)."
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
