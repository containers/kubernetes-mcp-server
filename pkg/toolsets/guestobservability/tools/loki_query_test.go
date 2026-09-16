package tools

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/guestobservability"
	"github.com/stretchr/testify/suite"
)

type LokiQuerySuite struct {
	suite.Suite
}

func TestLokiQuerySuite(t *testing.T) {
	suite.Run(t, new(LokiQuerySuite))
}

func (s *LokiQuerySuite) TestInitLokiQuery() {
	tools := InitLokiQuery()

	s.Require().Len(
		tools,
		1,
		"expected exactly one Loki tool",
	)

	serverTool := tools[0]
	tool := serverTool.Tool

	s.Run("metadata", func() {
		s.Run("uses expected tool name", func() {
			s.Equal(
				"guest-observability_loki_query",
				tool.Name,
				"unexpected Loki tool name",
			)
		})

		s.Run("has useful description", func() {
			s.NotEmpty(
				tool.Description,
				"expected non-empty tool description",
			)

			expectedFragments := []string{
				"LogQL range query",
				"namespace",
				"vm_name",
				"os",
				"source",
				"collector",
				"orphan guest telemetry",
				"label matchers exclude streams",
				"does not prove that matching guest events are absent",
				"progressively fewer contract-label matchers",
				"event itself",
				"should not be replaced with a log-content filter",
				"does not establish namespace association",
				"stable event identifiers",
				"case-insensitive matching",
				`os="windows"`,
				`source="windows_eventlog"`,
				"retained in the initial query",
				"Event ID 129",
				"StorPort",
				"event detection and VM attribution",
				"Reliable VM identity requires both namespace and vm_name",
				"classification uncertainty",
				"guest telemetry contract warnings",
			}

			for _, fragment := range expectedFragments {
				s.Contains(
					tool.Description,
					fragment,
					"expected tool description to contain %q",
					fragment,
				)
			}
		})
	})

	s.Run("input schema", func() {
		schema := tool.InputSchema

		s.Require().NotNil(
			schema,
			"expected input schema",
		)

		s.Run("requires query argument", func() {
			s.Contains(
				schema.Required,
				"query",
				"expected query to be required",
			)
		})

		s.Run("defines expected properties", func() {
			expected := []string{
				"query",
				"start",
				"end",
				"limit",
				"direction",
				"step",
			}

			for _, name := range expected {
				s.Contains(
					schema.Properties,
					name,
					"expected schema property %q",
					name,
				)
			}
		})

		s.Run("does not advertise unsupported since argument", func() {
			s.NotContains(
				schema.Properties,
				"since",
				"did not expect unsupported since property",
			)
		})

		s.Run("limit defaults to safe value", func() {
			property := schema.Properties["limit"]

			s.Require().NotNil(
				property,
				"limit property not found",
			)
			s.Require().NotNil(
				property.Default,
				"expected default limit",
			)

			s.Equal(
				"100",
				string(property.Default),
				"expected limit default %d",
				guestobservability.DefaultLokiLimit,
			)
		})

		s.Run("direction exposes forward and backward", func() {
			property := schema.Properties["direction"]

			s.Require().NotNil(
				property,
				"direction property not found",
			)

			s.Require().Len(
				property.Enum,
				2,
				"expected exactly two direction values",
			)

			values := map[string]bool{}

			for _, value := range property.Enum {
				if direction, ok := value.(string); ok {
					values[direction] = true
				}
			}

			s.True(
				values["forward"],
				"expected forward direction",
			)
			s.True(
				values["backward"],
				"expected backward direction",
			)
		})
	})

	s.Run("annotations", func() {
		annotations := tool.Annotations

		s.Run("tool is read only", func() {
			s.Require().NotNil(
				annotations.ReadOnlyHint,
				"expected read-only hint",
			)

			s.True(
				*annotations.ReadOnlyHint,
				"expected read-only hint to be true",
			)
		})

		s.Run("tool is non destructive", func() {
			s.Require().NotNil(
				annotations.DestructiveHint,
				"expected destructive hint",
			)

			s.False(
				*annotations.DestructiveHint,
				"expected non-destructive tool",
			)
		})

		s.Run("tool is idempotent", func() {
			s.Require().NotNil(
				annotations.IdempotentHint,
				"expected idempotent hint",
			)

			s.True(
				*annotations.IdempotentHint,
				"expected idempotent hint to be true",
			)
		})
	})

	s.Run("registration", func() {
		s.Run("handler is registered", func() {
			s.NotNil(
				serverTool.Handler,
				"expected Loki query handler",
			)
		})
	})
}
