package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type ConfirmationValidatorSuite struct {
	suite.Suite
	validator *ConfirmationValidator
}

func (s *ConfirmationValidatorSuite) SetupTest() {
	// Without an MCP session in the context elicitation is unsupported and the
	// "deny" fallback turns every matched rule into a validation error, which
	// is how the tests observe "confirmation was required".
	s.validator = &ConfirmationValidator{
		fallback: "deny",
		rules: []config.ConfirmationRule{{
			Kind:       "PipelineRun",
			Group:      "tekton.dev",
			Expression: "verb in ['create', 'update', 'patch'] && has(object.spec) && has(object.spec.pipelineRef) && object.spec.pipelineRef.name == 'windows-efi-installer'",
			Message:    "Windows golden image?",
		}},
	}
}

func (s *ConfirmationValidatorSuite) request(verb string, body string) *api.HTTPValidationRequest {
	return &api.HTTPValidationRequest{
		Verb:         verb,
		GVK:          &schema.GroupVersionKind{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"},
		Namespace:    "default",
		ResourceName: "run",
		Body:         []byte(body),
	}
}

func (s *ConfirmationValidatorSuite) TestExpressionRules() {
	s.Run("an object body that matches requires confirmation", func() {
		err := s.validator.Validate(s.T().Context(), s.request("create", `{"spec":{"pipelineRef":{"name":"windows-efi-installer"}}}`))
		s.Require().Error(err, "the request must be blocked until the user confirms")
	})
	s.Run("an object body that does not match passes", func() {
		s.NoError(s.validator.Validate(s.T().Context(), s.request("create", `{"spec":{"pipelineRef":{"name":"build"}}}`)))
	})
	s.Run("a JSON patch array is not an object and passes the has() guard", func() {
		// The rule cannot inspect a JSON patch; it must not fail closed and
		// challenge every such patch with an evaluation error either.
		s.NoError(s.validator.Validate(s.T().Context(), s.request("patch", `[{"op":"replace","path":"/spec/status","value":"Cancelled"}]`)))
	})
	s.Run("a request without a body passes the has() guard", func() {
		s.NoError(s.validator.Validate(s.T().Context(), s.request("delete", "")))
	})
}

func TestConfirmationValidator(t *testing.T) {
	suite.Run(t, new(ConfirmationValidatorSuite))
}
