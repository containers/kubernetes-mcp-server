package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAddGuestTelemetryContractWarnings(t *testing.T) {
	t.Run("complete contract leaves response unchanged", func(t *testing.T) {
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

		if got != input {
			t.Fatalf(
				"expected complete-contract response to remain unchanged\n"+
					"got: %s",
				got,
			)
		}
	})

	t.Run("missing vm_name makes VM identity unreliable", func(t *testing.T) {
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

		warnings := contractWarningsFromResponse(
			t,
			addGuestTelemetryContractWarnings(input),
		)

		if len(warnings) != 1 {
			t.Fatalf(
				"expected 1 warning, got %d",
				len(warnings),
			)
		}

		warning := warnings[0]

		if warning.IdentityReliable {
			t.Fatal(
				"expected identity to be unreliable when vm_name is missing",
			)
		}

		if !containsString(warning.Missing, "vm_name") {
			t.Fatalf(
				"expected vm_name in missing labels: %#v",
				warning.Missing,
			)
		}

		expectedFragments := []string{
			"The affected VM is UNKNOWN",
			"Do not name, rank, suggest, or speculate",
			`"most likely" or "strongest candidate"`,
			"Do not infer VM identity from VM names",
		}

		for _, fragment := range expectedFragments {
			if !strings.Contains(
				warning.Message,
				fragment,
			) {
				t.Fatalf(
					"expected warning to contain %q; got %q",
					fragment,
					warning.Message,
				)
			}
		}
	})

	t.Run("missing namespace makes VM identity unreliable", func(t *testing.T) {
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

		warnings := contractWarningsFromResponse(
			t,
			addGuestTelemetryContractWarnings(input),
		)

		if len(warnings) != 1 {
			t.Fatalf(
				"expected 1 warning, got %d",
				len(warnings),
			)
		}

		warning := warnings[0]

		if warning.IdentityReliable {
			t.Fatal(
				"expected identity to be unreliable when namespace is missing",
			)
		}

		if !strings.Contains(
			warning.Message,
			"vm_name alone does not identify a namespaced VM",
		) {
			t.Fatalf(
				"unexpected warning: %q",
				warning.Message,
			)
		}
	})

	t.Run("missing both identity labels reports VM and namespace unknown", func(t *testing.T) {
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

		warnings := contractWarningsFromResponse(
			t,
			addGuestTelemetryContractWarnings(input),
		)

		if len(warnings) != 1 {
			t.Fatalf(
				"expected 1 warning, got %d",
				len(warnings),
			)
		}

		warning := warnings[0]

		if warning.IdentityReliable {
			t.Fatal(
				"expected identity to be unreliable",
			)
		}

		if !containsString(warning.Missing, "namespace") {
			t.Fatalf(
				"expected namespace in missing labels: %#v",
				warning.Missing,
			)
		}

		if !containsString(warning.Missing, "vm_name") {
			t.Fatalf(
				"expected vm_name in missing labels: %#v",
				warning.Missing,
			)
		}

		if !strings.Contains(
			warning.Message,
			"The affected VM and namespace are UNKNOWN",
		) {
			t.Fatalf(
				"unexpected warning: %q",
				warning.Message,
			)
		}
	})

	t.Run("missing classification label keeps identity reliable", func(t *testing.T) {
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

		warnings := contractWarningsFromResponse(
			t,
			addGuestTelemetryContractWarnings(input),
		)

		if len(warnings) != 1 {
			t.Fatalf(
				"expected 1 warning, got %d",
				len(warnings),
			)
		}

		warning := warnings[0]

		if !warning.IdentityReliable {
			t.Fatal(
				"expected identity to remain reliable when only os is missing",
			)
		}

		if !containsString(warning.Missing, "os") {
			t.Fatalf(
				"expected os in missing labels: %#v",
				warning.Missing,
			)
		}

		if !strings.Contains(
			warning.Message,
			"Preserve this classification uncertainty",
		) {
			t.Fatalf(
				"unexpected warning: %q",
				warning.Message,
			)
		}
	})

	t.Run("duplicate missing-label combinations are deduplicated", func(t *testing.T) {
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

		warnings := contractWarningsFromResponse(
			t,
			addGuestTelemetryContractWarnings(input),
		)

		if len(warnings) != 1 {
			t.Fatalf(
				"expected duplicate warnings to be deduplicated, got %d",
				len(warnings),
			)
		}
	})

	t.Run("invalid JSON is returned unchanged", func(t *testing.T) {
		input := "not-json"

		got := addGuestTelemetryContractWarnings(input)

		if got != input {
			t.Fatalf(
				"expected invalid JSON to remain unchanged; got %q",
				got,
			)
		}
	})
}

func contractWarningsFromResponse(
	t *testing.T,
	content string,
) []guestTelemetryContractWarning {
	t.Helper()

	var response struct {
		Warnings []guestTelemetryContractWarning `json:"guestTelemetryContractWarnings"`
	}

	if err := json.Unmarshal(
		[]byte(content),
		&response,
	); err != nil {
		t.Fatalf(
			"failed to decode enriched Loki response: %v\nresponse: %s",
			err,
			content,
		)
	}

	return response.Warnings
}
