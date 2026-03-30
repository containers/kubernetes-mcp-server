package confirmation

import (
	"context"
	"errors"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

type mockElicitor struct {
	result *api.ElicitResult
	err    error
}

func (m *mockElicitor) Elicit(_ context.Context, _ *api.ElicitParams) (*api.ElicitResult, error) {
	return m.result, m.err
}

type ConfirmationSuite struct {
	suite.Suite
}

func (s *ConfirmationSuite) TestCheckConfirmation() {
	ctx := context.Background()

	s.Run("returns nil on accept", func() {
		elicitor := &mockElicitor{result: &api.ElicitResult{Action: api.ElicitActionAccept}}
		err := CheckConfirmation(ctx, elicitor, "confirm?", "deny")
		s.NoError(err)
	})
	s.Run("returns error on decline", func() {
		elicitor := &mockElicitor{result: &api.ElicitResult{Action: api.ElicitActionDecline}}
		err := CheckConfirmation(ctx, elicitor, "confirm?", "deny")
		s.ErrorIs(err, ErrConfirmationDenied)
	})
	s.Run("returns error on cancel", func() {
		elicitor := &mockElicitor{result: &api.ElicitResult{Action: api.ElicitActionCancel}}
		err := CheckConfirmation(ctx, elicitor, "confirm?", "deny")
		s.ErrorIs(err, ErrConfirmationDenied)
	})
	s.Run("elicitation not supported with deny fallback returns error", func() {
		elicitor := &mockElicitor{err: api.ErrElicitationNotSupported}
		err := CheckConfirmation(ctx, elicitor, "confirm?", "deny")
		s.ErrorIs(err, ErrConfirmationDenied)
	})
	s.Run("elicitation not supported with allow fallback returns nil", func() {
		elicitor := &mockElicitor{err: api.ErrElicitationNotSupported}
		err := CheckConfirmation(ctx, elicitor, "confirm?", "allow")
		s.NoError(err)
	})
	s.Run("wrapped elicitation not supported error is detected", func() {
		wrapped := errors.Join(api.ErrElicitationNotSupported, errors.New("extra context"))
		elicitor := &mockElicitor{err: wrapped}
		err := CheckConfirmation(ctx, elicitor, "confirm?", "deny")
		s.ErrorIs(err, ErrConfirmationDenied)
	})
	s.Run("other errors are returned as-is", func() {
		otherErr := errors.New("network failure")
		elicitor := &mockElicitor{err: otherErr}
		err := CheckConfirmation(ctx, elicitor, "confirm?", "deny")
		s.ErrorIs(err, otherErr)
		s.NotErrorIs(err, ErrConfirmationDenied)
	})
}

func (s *ConfirmationSuite) TestCheckToolRules() {
	ctx := context.Background()

	s.Run("no matching rules returns nil", func() {
		provider := &mockProvider{rules: []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "uninstall"},
		}, fallback: "deny"}
		elicitor := &mockElicitor{result: &api.ElicitResult{Action: api.ElicitActionDecline}}
		err := CheckToolRules(ctx, provider, elicitor, "pods_list", nil)
		s.NoError(err)
	})
	s.Run("matching rule with accept returns nil", func() {
		provider := &mockProvider{rules: []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "uninstall"},
		}, fallback: "deny"}
		elicitor := &mockElicitor{result: &api.ElicitResult{Action: api.ElicitActionAccept}}
		err := CheckToolRules(ctx, provider, elicitor, "helm_uninstall", nil)
		s.NoError(err)
	})
	s.Run("matching rule with decline returns error", func() {
		provider := &mockProvider{rules: []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "uninstall"},
		}, fallback: "deny"}
		elicitor := &mockElicitor{result: &api.ElicitResult{Action: api.ElicitActionDecline}}
		err := CheckToolRules(ctx, provider, elicitor, "helm_uninstall", nil)
		s.ErrorIs(err, ErrConfirmationDenied)
	})
}

func (s *ConfirmationSuite) TestCheckKubeRules() {
	ctx := context.Background()

	s.Run("no matching rules returns nil", func() {
		provider := &mockProvider{rules: []api.ConfirmationRule{
			{Verb: "delete", Namespace: "kube-system", Message: "delete in kube-system"},
		}, fallback: "deny"}
		elicitor := &mockElicitor{result: &api.ElicitResult{Action: api.ElicitActionDecline}}
		err := CheckKubeRules(ctx, provider, elicitor, "get", "Pod", "", "v1", "", "default")
		s.NoError(err)
	})
	s.Run("matching rule with accept returns nil", func() {
		provider := &mockProvider{rules: []api.ConfirmationRule{
			{Verb: "delete", Namespace: "kube-system", Message: "delete in kube-system"},
		}, fallback: "deny"}
		elicitor := &mockElicitor{result: &api.ElicitResult{Action: api.ElicitActionAccept}}
		err := CheckKubeRules(ctx, provider, elicitor, "delete", "Pod", "", "v1", "", "kube-system")
		s.NoError(err)
	})
	s.Run("matching rule with decline returns error", func() {
		provider := &mockProvider{rules: []api.ConfirmationRule{
			{Verb: "delete", Namespace: "kube-system", Message: "delete in kube-system"},
		}, fallback: "deny"}
		elicitor := &mockElicitor{result: &api.ElicitResult{Action: api.ElicitActionDecline}}
		err := CheckKubeRules(ctx, provider, elicitor, "delete", "Pod", "", "v1", "", "kube-system")
		s.ErrorIs(err, ErrConfirmationDenied)
	})
}

type mockProvider struct {
	rules    []api.ConfirmationRule
	fallback string
}

func (m *mockProvider) GetConfirmationRules() []api.ConfirmationRule { return m.rules }
func (m *mockProvider) GetConfirmationFallback() string              { return m.fallback }

func TestConfirmation(t *testing.T) {
	suite.Run(t, new(ConfirmationSuite))
}
