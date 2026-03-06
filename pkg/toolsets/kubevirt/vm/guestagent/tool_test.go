package guestagent

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type GuestAgentToolSuite struct {
	suite.Suite
}

func (s *GuestAgentToolSuite) TestToolRegistration() {
	s.Run("tool is registered", func() {
		tools := Tools()
		s.Require().Len(tools, 1, "Expected 1 guest agent tool")
		s.Equal("vm_guest_info", tools[0].Tool.Name)
		s.Equal("Virtual Machine: Guest Agent Info", tools[0].Tool.Annotations.Title)
		s.NotNil(tools[0].Tool.InputSchema)
		s.NotNil(tools[0].Handler)
	})

	s.Run("tool has correct properties", func() {
		tools := Tools()
		tool := tools[0].Tool

		// Check annotations
		s.True(*tool.Annotations.ReadOnlyHint, "guest info should be read-only")
		s.False(*tool.Annotations.DestructiveHint, "guest info should not be destructive")
		s.True(*tool.Annotations.IdempotentHint, "guest info should be idempotent")

		// Check schema
		schema := tool.InputSchema
		s.Require().NotNil(schema.Properties)
		s.Contains(schema.Properties, "namespace")
		s.Contains(schema.Properties, "name")
		s.Contains(schema.Properties, "info_type")

		// Check required fields
		s.ElementsMatch([]string{"namespace", "name"}, schema.Required)

		// Check info_type enum values
		infoTypeSchema := schema.Properties["info_type"]
		s.Require().NotNil(infoTypeSchema)
		s.ElementsMatch([]any{"all", "os", "filesystem", "users", "network"}, infoTypeSchema.Enum)
	})
}

func (s *GuestAgentToolSuite) TestInfoTypeConstants() {
	s.Run("info type constants are defined", func() {
		s.Equal(GuestAgentInfoType("all"), InfoTypeAll)
		s.Equal(GuestAgentInfoType("os"), InfoTypeOS)
		s.Equal(GuestAgentInfoType("filesystem"), InfoTypeFilesystem)
		s.Equal(GuestAgentInfoType("users"), InfoTypeUsers)
		s.Equal(GuestAgentInfoType("network"), InfoTypeNetwork)
	})
}

func TestGuestAgentToolSuite(t *testing.T) {
	suite.Run(t, new(GuestAgentToolSuite))
}
