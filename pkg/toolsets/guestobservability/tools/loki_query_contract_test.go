package tools

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type LokiQueryContractSuite struct {
	suite.Suite
}

func TestLokiQueryContractSuite(t *testing.T) {
	suite.Run(t, new(LokiQueryContractSuite))
}

func (s *LokiQueryContractSuite) TestAddGuestTelemetryContractWarnings() {
	s.Run("complete contract", func() {
		s.Run("leaves response unchanged", func() {
			input := `{
"status":"success",
"data":{
"resultType":"streams",
"result":[
{
"stream":{
"namespace":"guest-observability-eval",
"vm_name":"win-event-129",
"os":"windows",
"source":"windows_eventlog"
},
"values":[["1","event"]]
}
]
}
}`

			result := lokiQueryResult(input)

			s.Require().NotNil(
				result,
				"expected Loki tool result",
			)
			s.Require().NotNil(
				result.StructuredContent,
				"expected structured Loki result",
			)
			s.JSONEq(
				input,
				result.Content,
				"expected complete-contract response content to remain equivalent",
			)

			structured, ok := result.StructuredContent.(map[string]any)
			s.Require().True(
				ok,
				"expected structured Loki result, got %T",
				result.StructuredContent,
			)

			s.NotContains(
				structured,
				"guestTelemetryContractWarnings",
				"did not expect contract warnings for complete telemetry",
			)
		})
	})

	s.Run("missing identity labels", func() {
		s.Run("missing vm_name makes VM identity unreliable", func() {
			input := `{
"status":"success",
"data":{
"resultType":"streams",
"result":[
{
"stream":{
"namespace":"guest-observability-eval",
"os":"windows",
"source":"windows_eventlog"
},
"values":[
[
"1",
"Provider=storport EventID=129 Reset to device"
]
]
}
]
}
}`

			warnings := s.contractWarningsFromResult(input)

			s.Require().Len(
				warnings,
				1,
				"expected exactly one contract warning",
			)

			warning := warnings[0]

			s.False(
				warning.IdentityReliable,
				"expected identity to be unreliable when vm_name is missing",
			)
			s.Contains(
				warning.Missing,
				"vm_name",
				"expected vm_name in missing labels",
			)

			expectedFragments := []string{
				"The affected VM is UNKNOWN",
				"Do not name, rank, suggest, or speculate",
				`"most likely" or "strongest candidate"`,
				"Do not infer VM identity from VM names",
			}

			for _, fragment := range expectedFragments {
				s.Contains(
					warning.Message,
					fragment,
					"expected warning to contain %q",
					fragment,
				)
			}
		})

		s.Run("missing namespace makes VM identity unreliable", func() {
			input := `{
"status":"success",
"data":{
"resultType":"streams",
"result":[
{
"stream":{
"vm_name":"win-event-129",
"os":"windows",
"source":"windows_eventlog"
},
"values":[["1","event"]]
}
]
}
}`

			warnings := s.contractWarningsFromResult(input)

			s.Require().Len(
				warnings,
				1,
				"expected exactly one contract warning",
			)

			warning := warnings[0]

			s.False(
				warning.IdentityReliable,
				"expected identity to be unreliable when namespace is missing",
			)
			expectedFragments := []string{
				"vm_name alone does not identify a namespaced VM",
				"must be treated as unassociated",
				"same vm_name is not sufficient",
				"Do not describe the association as probable, likely, or inferred",
			}

			for _, fragment := range expectedFragments {
				s.Contains(
					warning.Message,
					fragment,
					"expected warning to contain %q",
					fragment,
				)
			}
		})

		s.Run("missing both identity labels reports VM and namespace unknown", func() {
			input := `{
"status":"success",
"data":{
"resultType":"streams",
"result":[
{
"stream":{
"collector":"guest-agent"
},
"values":[["1","event"]]
}
]
}
}`

			warnings := s.contractWarningsFromResult(input)

			s.Require().Len(
				warnings,
				1,
				"expected exactly one contract warning",
			)

			warning := warnings[0]

			s.False(
				warning.IdentityReliable,
				"expected identity to be unreliable",
			)
			s.Contains(
				warning.Missing,
				"namespace",
				"expected namespace in missing labels",
			)
			s.Contains(
				warning.Missing,
				"vm_name",
				"expected vm_name in missing labels",
			)
			s.Contains(
				warning.Message,
				"The affected VM and namespace are UNKNOWN",
				"unexpected warning",
			)
		})
	})

	s.Run("missing classification labels", func() {
		s.Run("keeps identity reliable", func() {
			input := `{
"status":"success",
"data":{
"resultType":"streams",
"result":[
{
"stream":{
"namespace":"guest-observability-eval",
"vm_name":"win-event-129",
"source":"windows_eventlog"
},
"values":[["1","event"]]
}
]
}
}`

			warnings := s.contractWarningsFromResult(input)

			s.Require().Len(
				warnings,
				1,
				"expected exactly one contract warning",
			)

			warning := warnings[0]

			s.True(
				warning.IdentityReliable,
				"expected identity to remain reliable when only os is missing",
			)
			s.Contains(
				warning.Missing,
				"os",
				"expected os in missing labels",
			)
			s.Contains(
				warning.Message,
				"Preserve this classification uncertainty",
				"unexpected warning",
			)
		})
	})

	s.Run("warning handling", func() {
		s.Run("duplicate missing-label combinations are deduplicated", func() {
			input := `{
"status":"success",
"data":{
"resultType":"streams",
"result":[
{
"stream":{
"namespace":"guest-observability-eval",
"os":"windows",
"source":"windows_eventlog"
},
"values":[["1","event one"]]
},
{
"stream":{
"namespace":"guest-observability-eval",
"os":"windows",
"source":"windows_eventlog"
},
"values":[["2","event two"]]
}
]
}
}`

			warnings := s.contractWarningsFromResult(input)

			s.Len(
				warnings,
				1,
				"expected duplicate warnings to be deduplicated",
			)
		})
		s.Run("invalid JSON falls back to text result", func() {
			input := "not-json"

			result := lokiQueryResult(input)

			s.Require().NotNil(
				result,
				"expected Loki tool result",
			)
			s.Equal(
				input,
				result.Content,
				"expected invalid JSON to fall back to text result",
			)
			s.Nil(
				result.StructuredContent,
				"did not expect structured content for invalid JSON",
			)
			s.Nil(
				result.Error,
				"did not expect Loki result error",
			)
		})
	})
}

func (s *LokiQueryContractSuite) contractWarningsFromResult(
	content string,
) []guestTelemetryContractWarning {
	s.T().Helper()

	result := lokiQueryResult(content)

	s.Require().NotNil(
		result,
		"expected Loki tool result",
	)
	s.Require().Nil(
		result.Error,
		"did not expect Loki result error",
	)

	structured, ok := result.StructuredContent.(map[string]any)
	s.Require().True(
		ok,
		"expected structured Loki result, got %T",
		result.StructuredContent,
	)

	rawWarnings, ok := structured["guestTelemetryContractWarnings"]
	if !ok {
		return nil
	}

	warnings, ok := rawWarnings.([]guestTelemetryContractWarning)
	s.Require().True(
		ok,
		"expected typed guest telemetry warnings, got %T",
		rawWarnings,
	)

	return warnings
}
