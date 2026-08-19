//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type clientsState struct {
	dep *serverDeployment
}

var clientsTS testState[clientsState]

func TestClientConnectivity(t *testing.T) {
	f := features.New("client-connectivity").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "clients",
				withConfig(`read_only = true`),
				withValues(viewClusterRoleBindingValues()),
			)
			return clientsTS.set(ctx, &clientsState{dep: dep})
		}).
		Assess("sdk matrix", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := clientsTS.get(ctx)

			type clientInfoCase struct {
				name   string
				option test.McpClientOption // nil means use SDK default
			}
			clientInfos := []clientInfoCase{
				{"named", test.WithClientInfo("test-client", "1.0")},
				{"empty", test.WithEmptyClientInfo()},
				{"default", nil},
			}

			type capCase struct {
				name   string
				option test.McpClientOption
			}
			capabilities := []capCase{
				{"none", test.WithClientCapabilities(&mcp.ClientCapabilities{})},
				{"roots", test.WithClientCapabilities(&mcp.ClientCapabilities{
					RootsV2: &mcp.RootCapabilities{ListChanged: true},
				})},
				{"elicitation", test.WithClientCapabilities(&mcp.ClientCapabilities{
					Elicitation: &mcp.ElicitationCapabilities{
						Form: &mcp.FormElicitationCapabilities{},
					},
				})},
				{"both", test.WithClientCapabilities(&mcp.ClientCapabilities{
					RootsV2: &mcp.RootCapabilities{ListChanged: true},
					Elicitation: &mcp.ElicitationCapabilities{
						Form: &mcp.FormElicitationCapabilities{},
					},
				})},
			}

			for _, ci := range clientInfos {
				for _, cap := range capabilities {
					name := fmt.Sprintf("%s/%s", ci.name, cap.name)
					opts := []test.McpClientOption{test.WithEndpoint(s.dep.serverURL + "/mcp")}
					if ci.option != nil {
						opts = append(opts, ci.option)
					}
					if cap.option != nil {
						opts = append(opts, cap.option)
					}
					t.Run(name, func(t *testing.T) {
						mcpClient := test.NewMcpClient(t, nil, opts...)
						t.Cleanup(mcpClient.Close)

						tools, err := mcpClient.ListTools()
						require.NoError(t, err, "ListTools")
						require.Greater(t, len(tools.Tools), 0, "expected at least one tool")

						output := requireToolCallSuccess(t, mcpClient, "configuration_view", nil)
						require.NotEmpty(t, output, "configuration_view returned empty")
					})
				}
			}

			return ctx
		}).
		Assess("protocol versions", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := clientsTS.get(ctx)
			endpoint := s.dep.serverURL + "/mcp"

			versions := []string{
				"2024-11-05",
				"2025-03-26",
				"2025-06-18",
				"2025-11-25",
			}

			for _, version := range versions {
				t.Run(version, func(t *testing.T) {
					sessionID, negotiated := rawInitialize(t, endpoint, version)
					require.Equal(t, version, negotiated, "server should negotiate requested version")
					require.NotEmpty(t, sessionID, "server should return a session ID")
					t.Cleanup(func() { rawDeleteSession(t, endpoint, sessionID) })

					rawNotify(t, endpoint, sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

					toolsBody := rawRequest(t, endpoint, sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
					require.Contains(t, toolsBody, `"tools"`, "tools/list response should contain tools")
					require.Contains(t, toolsBody, `"configuration_view"`, "tools/list should include configuration_view")

					callBody := rawRequest(t, endpoint, sessionID,
						`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"configuration_view","arguments":{}}}`)
					require.Contains(t, callBody, `"content"`, "tools/call response should contain content")
					require.NotContains(t, callBody, `"isError":true`, "tools/call should not return an error")
				})
			}

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

// rawInitialize sends a raw JSON-RPC initialize request with the given protocol
// version and returns the session ID and the server's negotiated version.
func rawInitialize(t *testing.T, endpoint, protocolVersion string) (sessionID, negotiatedVersion string) {
	t.Helper()
	body := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":{},"clientInfo":{"name":"e2e-proto","version":"0"}}}`,
		protocolVersion,
	)
	resp := test.McpRawPost(t, endpoint, "", body)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "initialize should return 200")

	sessionID = resp.Header.Get("Mcp-Session-Id")

	jsonBody := readEventStreamData(t, resp.Body)

	var parsed struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonBody), &parsed), "parse initialize response")
	return sessionID, parsed.Result.ProtocolVersion
}

// rawNotify sends a JSON-RPC notification (no id, no response expected).
func rawNotify(t *testing.T, endpoint, sessionID, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
}

// rawRequest sends a raw JSON-RPC request and returns the response body as a string.
func rawRequest(t *testing.T, endpoint, sessionID, body string) string {
	t.Helper()
	resp := test.McpRawPost(t, endpoint, sessionID, body)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return readEventStreamData(t, resp.Body)
}

// readEventStreamData extracts the JSON payload from a streamable HTTP response.
// The response uses event-stream framing: "event: message\ndata: {json}\n\n".
func readEventStreamData(t *testing.T, r io.Reader) string {
	t.Helper()
	var result string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if result == "" {
			if after, ok := strings.CutPrefix(line, "data: "); ok {
				result = after
			}
		}
	}
	require.NoError(t, scanner.Err(), "scan event-stream response")
	require.NotEmpty(t, result, "no data line found in event-stream response")
	return result
}

// rawDeleteSession sends an HTTP DELETE to cleanly terminate a raw MCP session,
// preventing port-forwarder "connection reset by peer" noise.
func rawDeleteSession(t *testing.T, endpoint, sessionID string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return
	}
	req.Header.Set("Mcp-Session-Id", sessionID)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
