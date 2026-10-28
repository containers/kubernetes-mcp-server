package traces

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOTLPTraceFlattensAttributes(t *testing.T) {
	const input = `{
		"resourceSpans": [{
			"resource": {
				"attributes": [
					{"key": "service.name", "value": {"stringValue": "frontend"}},
					{"key": "process.pid", "value": {"intValue": "1234"}}
				]
			},
			"scopeSpans": [{
				"scope": {
					"name": "tracer",
					"attributes": [{"key": "version", "value": {"stringValue": "1.0"}}]
				},
				"spans": [{
					"name": "GET /",
					"traceId": "AAECAwQFBgcICQoLDA0ODw==",
					"spanId": "AQIDBAUGBwg=",
					"attributes": [
						{"key": "http.method", "value": {"stringValue": "GET"}},
						{"key": "http.status_code", "value": {"intValue": "200"}},
						{"key": "error", "value": {"boolValue": true}},
						{"key": "ratio", "value": {"doubleValue": 0.5}},
						{"key": "tags", "value": {"arrayValue": {"values": [{"stringValue": "a"}, {"stringValue": "b"}]}}},
						{"key": "meta", "value": {"kvlistValue": {"values": [{"key": "region", "value": {"stringValue": "us"}}]}}}
					]
				}]
			}]
		}]
	}`

	var trace any
	require.NoError(t, json.Unmarshal([]byte(input), &trace))

	formatTrace(trace)

	output, err := json.Marshal(trace)
	require.NoError(t, err)

	require.JSONEq(t, `{
		"resourceSpans": [{
			"resource": {"attributes": {"service.name": "frontend", "process.pid": "1234"}},
			"scopeSpans": [{
				"scope": {"name": "tracer", "attributes": {"version": "1.0"}},
				"spans": [{
					"name": "GET /",
					"traceId": "000102030405060708090a0b0c0d0e0f",
					"spanId": "0102030405060708",
					"attributes": {
						"http.method": "GET",
						"http.status_code": "200",
						"error": true,
						"ratio": 0.5,
						"tags": ["a", "b"],
						"meta": {"region": "us"}
					}
				}]
			}]
		}]
	}`, string(output))
}
