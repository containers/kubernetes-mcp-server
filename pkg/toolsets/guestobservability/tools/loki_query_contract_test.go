package tools

import (
	"encoding/json"
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

			got := addGuestTelemetryContractWarnings(input)

			s.Equal(
				input,
				got,
				"expected complete-contract response to remain unchanged",
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

			warnings := s.contractWarningsFromResponse(
				addGuestTelemetryContractWarnings(input),
			)

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

			warnings := s.contractWarningsFromResponse(
				addGuestTelemetryContractWarnings(input),
			)

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
			s.Contains(
				warning.Message,
				"vm_name alone does not identify a namespaced VM",
				"unexpected warning",
			)
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

			warnings := s.contractWarningsFromResponse(
				addGuestTelemetryContractWarnings(input),
			)

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

			warnings := s.contractWarningsFromResponse(
				addGuestTelemetryContractWarnings(input),
			)

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

			warnings := s.contractWarningsFromResponse(
				addGuestTelemetryContractWarnings(input),
			)

			s.Len(
				warnings,
				1,
				"expected duplicate warnings to be deduplicated",
			)
		})

		s.Run("invalid JSON is returned unchanged", func() {
			input := "not-json"

			got := addGuestTelemetryContractWarnings(input)

			s.Equal(
				input,
				got,
				"expected invalid JSON to remain unchanged",
			)
		})
	})
}

func (s *LokiQueryContractSuite) contractWarningsFromResponse(
	content string,
) []guestTelemetryContractWarning {
	s.T().Helper()

	var response struct {
		Warnings []guestTelemetryContractWarning `json:"guestTelemetryContractWarnings"`
	}

	err := json.Unmarshal(
		[]byte(content),
		&response,
	)

	s.Require().NoError(
		err,
		"failed to decode enriched Loki response: %s",
		content,
	)

	return response.Warnings
}
