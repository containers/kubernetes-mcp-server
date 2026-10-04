package defaults

const (
	DefaultToolsetName = "guest-observability"

	DefaultToolsetDescription = "Virtual machine guest log observability tools backed by Loki. Check the [Guest Observability documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/guest-observability.md) for more details."
)

func ToolsetName() string {
	return DefaultToolsetName
}

func ToolsetDescription() string {
	return DefaultToolsetDescription
}
