package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsVerboseEnabled(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"true lowercase", "true", true},
		{"true capitalized", "True", true},
		{"true uppercase", "TRUE", true},
		{"numeric 1", "1", true},
		{"yes lowercase", "yes", true},
		{"yes capitalized", "Yes", true},
		{"yes uppercase", "YES", true},
		{"y lowercase", "y", true},
		{"y uppercase", "Y", true},
		{"false", "false", false},
		{"0", "0", false},
		{"no", "no", false},
		{"empty string", "", false},
		{"random string", "random", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVerboseEnabled(tt.input)
			assert.Equal(t, tt.expected, result, "isVerboseEnabled(%q) should return %v", tt.input, tt.expected)
		})
	}
}

func TestInitHealthCheckPrompts(t *testing.T) {
	// When
	prompts := initHealthCheckPrompts()

	// Then
	require.Len(t, prompts, 1)
	assert.Equal(t, "cluster_health_check", prompts[0].Name)
	assert.Contains(t, prompts[0].Description, "comprehensive health check")
	assert.Len(t, prompts[0].Arguments, 4)

	// Check arguments
	assert.Equal(t, "check_events", prompts[0].Arguments[0].Name)
	assert.False(t, prompts[0].Arguments[0].Required)

	assert.Equal(t, "output_format", prompts[0].Arguments[1].Name)
	assert.False(t, prompts[0].Arguments[1].Required)

	assert.Equal(t, "verbose", prompts[0].Arguments[2].Name)
	assert.False(t, prompts[0].Arguments[2].Required)

	assert.Equal(t, "namespace", prompts[0].Arguments[3].Name)
	assert.False(t, prompts[0].Arguments[3].Required)
}

func TestBuildHealthCheckPromptMessages(t *testing.T) {
	t.Run("Default messages with no arguments", func(t *testing.T) {
		// When - checkEvents=true (default), outputFormat="text" (default)
		messages := buildHealthCheckPromptMessages(false, "", true, "text")

		// Then
		require.Len(t, messages, 2)
		assert.Equal(t, "user", messages[0].Role)
		assert.Equal(t, "assistant", messages[1].Role)

		// Check user message content
		userContent := messages[0].Content
		assert.Contains(t, userContent, "across all namespaces")
		assert.Contains(t, userContent, "Use pods_list to get all pods")
		assert.Contains(t, userContent, "resources_list")
		assert.Contains(t, userContent, "Check Recent Events")
		assert.NotContains(t, userContent, "pods_list_in_namespace")

		// Check assistant message
		assert.Contains(t, messages[1].Content, "comprehensive cluster health check")
	})

	t.Run("Messages with namespace filter", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(false, "test-namespace", true, "text")

		// Then
		require.Len(t, messages, 2)

		userContent := messages[0].Content
		assert.Contains(t, userContent, "in namespace 'test-namespace'")
		assert.NotContains(t, userContent, "across all namespaces")
		assert.Contains(t, userContent, "Use pods_list_in_namespace with namespace 'test-namespace'")
		assert.NotContains(t, userContent, "Use pods_list to get all pods")
	})

	t.Run("Messages with verbose mode", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(true, "", true, "text")

		// Then
		require.Len(t, messages, 2)

		userContent := messages[0].Content
		assert.Contains(t, userContent, "For verbose mode")
		assert.Contains(t, userContent, "Specific error messages")
		assert.Contains(t, userContent, "Resource-level details")
		assert.Contains(t, userContent, "Individual pod and deployment names")
	})

	t.Run("Messages with both verbose and namespace", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(true, "prod", true, "text")

		// Then
		require.Len(t, messages, 2)

		userContent := messages[0].Content
		assert.Contains(t, userContent, "in namespace 'prod'")
		assert.Contains(t, userContent, "For verbose mode")
	})

	t.Run("Messages with checkEvents disabled", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(false, "", false, "text")

		// Then
		require.Len(t, messages, 2)

		userContent := messages[0].Content
		assert.NotContains(t, userContent, "Check Recent Events")
		assert.NotContains(t, userContent, "events_list")
	})

	t.Run("Messages with JSON output format", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(false, "", true, "json")

		// Then
		require.Len(t, messages, 2)

		userContent := messages[0].Content
		assert.Contains(t, userContent, "JSON object")
		assert.Contains(t, userContent, "cluster_type")
		assert.Contains(t, userContent, "node_health")
		assert.Contains(t, userContent, "pod_health")
		assert.NotContains(t, userContent, "✅")
		assert.NotContains(t, userContent, "⚠️")
	})

	t.Run("User message contains all required sections", func(t *testing.T) {
		// When - with checkEvents enabled
		messages := buildHealthCheckPromptMessages(false, "", true, "text")

		// Then
		userContent := messages[0].Content

		// Check for all main sections
		sections := []string{
			"## 1. Check Cluster-Level Components",
			"## 2. Check Node Health",
			"## 3. Check Pod Health",
			"## 4. Check Workload Controllers",
			"## 5. Check Storage",
			"## 6. Check Recent Events",
			"## Output Format",
			"## Health Status Definitions",
			"## Important Notes",
		}

		for _, section := range sections {
			assert.Contains(t, userContent, section, "Missing section: %s", section)
		}
	})

	t.Run("User message contains critical tool references", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(false, "", true, "text")

		// Then
		userContent := messages[0].Content

		// Check for tool names
		tools := []string{
			"resources_list",
			"pods_list",
		}

		for _, tool := range tools {
			assert.Contains(t, userContent, tool, "Missing tool reference: %s", tool)
		}
	})

	t.Run("User message contains health check criteria", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(false, "", true, "text")

		// Then
		userContent := messages[0].Content

		// Check for critical conditions
		criteria := []string{
			"Degraded=True (CRITICAL)",
			"Available=False (CRITICAL)",
			"Ready condition != True (CRITICAL)",
			"CrashLoopBackOff (CRITICAL)",
			"ImagePullBackOff",
			"RestartCount > 5 (WARNING",
			"MemoryPressure",
			"DiskPressure",
		}

		for _, criterion := range criteria {
			assert.Contains(t, userContent, criterion, "Missing criterion: %s", criterion)
		}
	})

	t.Run("User message contains workload types with apiVersions", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(false, "", true, "text")

		// Then
		userContent := messages[0].Content

		// Check for apiVersion + kind pairs
		resourceSpecs := []string{
			"apiVersion=apps/v1, kind=Deployment",
			"apiVersion=apps/v1, kind=StatefulSet",
			"apiVersion=apps/v1, kind=DaemonSet",
			"apiVersion=config.openshift.io/v1 and kind=ClusterOperator",
			"apiVersion=v1 and kind=Node",
			"apiVersion=v1 and kind=PersistentVolumeClaim",
		}

		for _, spec := range resourceSpecs {
			assert.Contains(t, userContent, spec, "Missing resource spec: %s", spec)
		}
	})

	t.Run("User message contains output format template", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(false, "", true, "text")

		// Then
		userContent := messages[0].Content

		// Check for report structure
		reportElements := []string{
			"Cluster Health Check Report",
			"Cluster Type:",
			"### Cluster Operators",
			"### Node Health",
			"### Pod Health",
			"### Workload Controllers",
			"### Storage",
			"### Recent Events",
			"Summary",
			"Critical Issues:",
			"Warnings:",
		}

		for _, element := range reportElements {
			assert.Contains(t, userContent, element, "Missing report element: %s", element)
		}
	})

	t.Run("User message does not reference non-existent tools", func(t *testing.T) {
		// When
		messages := buildHealthCheckPromptMessages(false, "", true, "text")

		// Then
		userContent := messages[0].Content

		// Make sure we're not referencing the old tool name
		assert.NotContains(t, userContent, "pods_list_in_all_namespaces")
	})
}

func TestGetMessagesWithArguments(t *testing.T) {
	// Given
	prompts := initHealthCheckPrompts()
	require.Len(t, prompts, 1)

	getMessages := prompts[0].GetMessages

	t.Run("With no arguments", func(t *testing.T) {
		// When
		messages := getMessages(map[string]string{})

		// Then
		require.Len(t, messages, 2)
		userContent := messages[0].Content
		assert.Contains(t, userContent, "across all namespaces")
		assert.NotContains(t, userContent, "For verbose mode")
		// Default is checkEvents=true
		assert.Contains(t, userContent, "Check Recent Events")
	})

	t.Run("With verbose=true", func(t *testing.T) {
		// When
		messages := getMessages(map[string]string{"verbose": "true"})

		// Then
		require.Len(t, messages, 2)
		userContent := messages[0].Content
		assert.Contains(t, userContent, "For verbose mode")
	})

	t.Run("With verbose=false", func(t *testing.T) {
		// When
		messages := getMessages(map[string]string{"verbose": "false"})

		// Then
		require.Len(t, messages, 2)
		userContent := messages[0].Content
		assert.NotContains(t, userContent, "For verbose mode")
	})

	t.Run("With namespace", func(t *testing.T) {
		// When
		messages := getMessages(map[string]string{"namespace": "kube-system"})

		// Then
		require.Len(t, messages, 2)
		userContent := messages[0].Content
		assert.Contains(t, userContent, "in namespace 'kube-system'")
	})

	t.Run("With all arguments", func(t *testing.T) {
		// When
		messages := getMessages(map[string]string{
			"verbose":       "true",
			"namespace":     "default",
			"check_events":  "false",
			"output_format": "json",
		})

		// Then
		require.Len(t, messages, 2)
		userContent := messages[0].Content
		assert.Contains(t, userContent, "For verbose mode")
		assert.Contains(t, userContent, "in namespace 'default'")
		assert.NotContains(t, userContent, "Check Recent Events")
		assert.Contains(t, userContent, "JSON object")
	})

	t.Run("With check_events=false", func(t *testing.T) {
		// When
		messages := getMessages(map[string]string{"check_events": "false"})

		// Then
		require.Len(t, messages, 2)
		userContent := messages[0].Content
		assert.NotContains(t, userContent, "Check Recent Events")
	})

	t.Run("With output_format=json", func(t *testing.T) {
		// When
		messages := getMessages(map[string]string{"output_format": "json"})

		// Then
		require.Len(t, messages, 2)
		userContent := messages[0].Content
		assert.Contains(t, userContent, "JSON object")
		assert.Contains(t, userContent, "cluster_type")
	})
}

func TestHealthCheckPromptCompleteness(t *testing.T) {
	// This test ensures the prompt covers all essential aspects

	messages := buildHealthCheckPromptMessages(false, "", true, "text")
	userContent := messages[0].Content

	t.Run("Covers all Kubernetes resource types", func(t *testing.T) {
		resourceTypes := []string{
			"Node",
			"Pod",
			"Deployment",
			"StatefulSet",
			"DaemonSet",
			"PersistentVolumeClaim",
			"ClusterOperator", // OpenShift specific
		}

		for _, rt := range resourceTypes {
			assert.Contains(t, userContent, rt, "Missing resource type: %s", rt)
		}
	})

	t.Run("Provides clear severity levels", func(t *testing.T) {
		assert.Contains(t, userContent, "CRITICAL")
		assert.Contains(t, userContent, "WARNING")
		assert.Contains(t, userContent, "HEALTHY")
	})

	t.Run("Includes efficiency guidelines", func(t *testing.T) {
		assert.Contains(t, userContent, "Be efficient")
		assert.Contains(t, userContent, "don't call the same tool multiple times unnecessarily")
	})

	t.Run("Handles OpenShift gracefully", func(t *testing.T) {
		assert.Contains(t, userContent, "For OpenShift Clusters")
		assert.Contains(t, userContent, "For All Kubernetes Clusters")
		assert.Contains(t, userContent, "skip it gracefully")
	})

	t.Run("Instructions are clear and actionable", func(t *testing.T) {
		// Check that the prompt uses imperative language
		imperativeVerbs := []string{"Use", "Check", "Look for", "Verify", "Identify", "Compare"}
		foundVerbs := 0
		for _, verb := range imperativeVerbs {
			if strings.Contains(userContent, verb) {
				foundVerbs++
			}
		}
		assert.Greater(t, foundVerbs, 3, "Prompt should use clear imperative language")
	})

	t.Run("Includes apiVersion reference section", func(t *testing.T) {
		assert.Contains(t, userContent, "Common apiVersion Values")
		assert.Contains(t, userContent, "apiVersion=config.openshift.io/v1")
		assert.Contains(t, userContent, "apiVersion=apps/v1")
		assert.Contains(t, userContent, "apiVersion=v1")
		assert.Contains(t, userContent, "ClusterOperator, ClusterVersion")
	})
}

func TestIsBooleanEnabled(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		defaultValue bool
		expected     bool
	}{
		{"empty with default true", "", true, true},
		{"empty with default false", "", false, false},
		{"true lowercase", "true", false, true},
		{"true uppercase", "TRUE", false, true},
		{"false lowercase", "false", true, false},
		{"false uppercase", "FALSE", true, false},
		{"yes", "yes", false, true},
		{"no", "no", true, false},
		{"1", "1", false, true},
		{"0", "0", true, false},
		{"invalid with default true", "invalid", true, true},
		{"invalid with default false", "invalid", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBooleanEnabled(tt.input, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEmojiInstructions(t *testing.T) {
	t.Run("Returns emoji instructions for text format", func(t *testing.T) {
		result := getEmojiInstructions("text")
		assert.Contains(t, result, "✅")
		assert.Contains(t, result, "⚠️")
		assert.Contains(t, result, "❌")
	})

	t.Run("Returns empty for json format", func(t *testing.T) {
		result := getEmojiInstructions("json")
		assert.Empty(t, result)
	})

	t.Run("Returns emoji instructions for other formats", func(t *testing.T) {
		result := getEmojiInstructions("yaml")
		assert.Contains(t, result, "✅")
	})
}
