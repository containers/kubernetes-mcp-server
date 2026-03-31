package mcp

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ValidationBypassSuite verifies that the validation_enabled config flag
// controls whether validators run in the AccessControlRoundTripper.
//
// Both tests remove the test user's RBAC permissions, then call namespaces_list
// through the full MCP stack. The behavioral difference:
//   - validation_enabled = true → the RBAC validator in the RoundTripper
//     catches the denial before the request reaches the API server, returning
//     a PERMISSION_DENIED ValidationError.
//   - validation_enabled = false → no validators run, so the request reaches
//     the API server, which returns a 403 Forbidden error.
//
// namespaces_list is chosen because namespaces are cluster-scoped and therefore
// unaffected by any leftover namespace-scoped Roles from other test suites.
type ValidationBypassSuite struct {
	BaseMcpSuite
}

func (s *ValidationBypassSuite) TestValidationDisabledPassesToAPIServer() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		validation_enabled = false
	`), s.Cfg), "Expected to parse validation config")
	s.InitMcpClient()
	defer restoreAuth(s.T().Context())
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)
	_ = client.RbacV1().ClusterRoles().Delete(s.T().Context(), "allow-all", metav1.DeleteOptions{})

	s.Run("API server rejects with 403 Forbidden", func() {
		result, err := s.CallTool("namespaces_list", map[string]any{})
		s.NoError(err)
		s.Require().NotNil(result)
		s.True(result.IsError, "expected tool call to fail")
		text := result.Content[0].(*mcp.TextContent).Text
		s.Contains(text, "forbidden", "expected server-side 403 Forbidden error")
		s.NotContains(text, "PERMISSION_DENIED", "should not contain client-side validation error")
		s.NotContains(text, "Validation Error", "should not contain client-side validation error")
	})
}

func (s *ValidationBypassSuite) TestValidationEnabledCatchesRBACDenial() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		validation_enabled = true
	`), s.Cfg), "Expected to parse validation config")
	s.InitMcpClient()
	defer restoreAuth(s.T().Context())
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)
	_ = client.RbacV1().ClusterRoles().Delete(s.T().Context(), "allow-all", metav1.DeleteOptions{})

	s.Run("RBAC validator rejects before reaching API server", func() {
		result, err := s.CallTool("namespaces_list", map[string]any{})
		s.NoError(err)
		s.Require().NotNil(result)
		s.True(result.IsError, "expected tool call to fail")
		text := result.Content[0].(*mcp.TextContent).Text
		s.Contains(text, "PERMISSION_DENIED", "expected client-side RBAC validation error")
		s.Contains(text, "Validation Error", "expected client-side validation error wrapper")
		s.NotContains(text, "forbidden", "should not contain server-side 403 error")
	})
}

func TestValidationBypass(t *testing.T) {
	suite.Run(t, new(ValidationBypassSuite))
}
