package defaults

const (
	DefaultToolsetName = "guest-observability"

	DefaultToolsetDescription = "Guest observability tools for querying metrics and logs from virtual machine guest operating systems."
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
