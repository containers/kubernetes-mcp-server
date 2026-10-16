package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/tools/ownership"
)

func TestRenderIncludesApproversButNotReviewers(t *testing.T) {
	result, err := render(ownership.Config{
		DefaultApprovers: []string{"@Cali0707", "@manusa"},
		DefaultReviewers: []string{"@mjudeikis"},
		Areas: []ownership.Area{{
			Toolset:   "kiali",
			Paths:     []string{"/pkg/toolsets/kiali/", "/pkg/kiali/"},
			Approvers: []string{"@matzew", "@manusa"},
			Reviewers: []string{"@aljesusg"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"* @Cali0707 @manusa",
		"/pkg/toolsets/kiali/ @Cali0707 @manusa @matzew",
		"/pkg/kiali/ @Cali0707 @manusa @matzew",
	} {
		if !hasRule(string(result), line) {
			t.Errorf("generated CODEOWNERS missing %q", line)
		}
	}
	if !strings.Contains(string(result), "# toolset: kiali\n") {
		t.Fatal("missing toolset section")
	}
	if strings.Contains(string(result), "@aljesusg") || strings.Contains(string(result), "@mjudeikis") {
		t.Fatal("reviewer must not appear in CODEOWNERS")
	}
}

func TestRenderCombinesApproversOnSharedPaths(t *testing.T) {
	result, err := render(ownership.Config{
		DefaultApprovers: []string{"@Cali0707"},
		Areas: []ownership.Area{
			{Toolset: "logs", Paths: []string{"/pkg/obs/logs/", "/pkg/obs/auth/", "/pkg/obs/toolcfg/"}, Approvers: []string{"@manusa"}},
			{Toolset: "metrics", Paths: []string{"/pkg/obs/metrics/", "/pkg/obs/auth/", "/pkg/obs/toolcfg/"}, Approvers: []string{"@matzew"}},
			{Toolset: "otelcol", Paths: []string{"/pkg/obs/toolcfg/"}, Approvers: []string{"@nader-ziada"}},
			{Toolset: "traces", Paths: []string{"/pkg/obs/auth/", "/pkg/obs/toolcfg/"}, Approvers: []string{"@2uasimojo"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if line := "/pkg/obs/toolcfg/ @Cali0707 @manusa @matzew @nader-ziada @2uasimojo"; !hasRule(string(result), line) {
		t.Fatalf("missing combined shared path rule: %s", result)
	}
	if count := strings.Count(string(result), "/pkg/obs/toolcfg/"); count != 1 {
		t.Fatalf("expected one shared path rule, got %d", count)
	}
	if !strings.Contains(string(result), "# shared: logs, metrics, traces\n") ||
		!strings.Contains(string(result), "# shared: logs, metrics, otelcol, traces\n") {
		t.Fatalf("missing distinct overlap sections: %s", result)
	}
}

func TestRenderKeepsRolesScopedToAreas(t *testing.T) {
	result, err := render(ownership.Config{
		DefaultApprovers: []string{"@Cali0707"},
		Areas: []ownership.Area{
			{Paths: []string{"/pkg/toolsets/helm/"}, Approvers: []string{"@manusa"}},
			{Paths: []string{"/pkg/toolsets/kcp/"}, Reviewers: []string{"@manusa", "@mjudeikis"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(string(result), "/pkg/toolsets/helm/ @Cali0707 @manusa") ||
		!hasRule(string(result), "/pkg/toolsets/kcp/ @Cali0707") ||
		strings.Contains(string(result), "@mjudeikis") {
		t.Fatalf("approver roles leaked between areas: %s", result)
	}
}

func hasRule(content, expected string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.Join(strings.Fields(line), " ") == expected {
			return true
		}
	}
	return false
}

func TestLoadStrictJSON(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		valid   bool
	}{
		{"valid roles", `{"default_approvers":["@Cali0707"],"default_reviewers":["@manusa"],"areas":[{"toolset":"kcp","paths":["/pkg/kcp/"],"approvers":["@manusa"],"reviewers":["@mjudeikis"]}]}`, true},
		{"unknown field", `{"default_approvers":["@Cali0707"],"areas":[],"unexpected":true}`, false},
		{"trailing document", `{"default_approvers":["@Cali0707"]} {"default_approvers":["@manusa"]}`, false},
		{"YAML", "default_approvers:\n  - '@Cali0707'\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ownership.json")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			config, err := ownership.Load(path)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				if config.Areas[0].Reviewers[0] != "@mjudeikis" {
					t.Fatalf("missing reviewer role: %+v", config.Areas[0])
				}
			} else if err == nil {
				t.Fatal("expected invalid input to be rejected")
			}
		})
	}
}
