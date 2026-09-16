package defaults

const (
	DefaultToolsetName = "guest-observability"

	DefaultToolsetDescription = "Guest observability tools for querying metrics and logs from virtual machine guest operating systems."
)

func ToolsetName() string {
	return DefaultToolsetName
}

func ToolsetDescription() string {
	return DefaultToolsetDescription
}
