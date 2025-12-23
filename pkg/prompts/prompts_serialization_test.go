package prompts

import (
	"encoding/json"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

// PromptSerializationSuite tests serialization of prompt data structures
type PromptSerializationSuite struct {
	suite.Suite
}

func (s *PromptSerializationSuite) TestPromptJSONSerialization() {
	prompt := api.Prompt{
		Name:        "test-prompt",
		Title:       "Test Prompt",
		Description: "A test prompt",
		Arguments: []api.PromptArgument{
			{Name: "arg1", Description: "First argument", Required: true},
			{Name: "arg_opt", Description: "Optional argument", Required: false},
		},
		Templates: []api.PromptTemplate{
			{Role: "user", Content: "Hello {{arg1}}"},
			{Role: "assistant", Content: "How can I assist you with {{arg1}}?"},
		},
	}
	jsonData := `{
			"name": "test-prompt",
			"title": "Test Prompt",
			"description": "A test prompt",
			"arguments": [
				{
					"name": "arg1",
					"description": "First argument",
					"required": true
				},
				{
					"name": "arg_opt",
					"description": "Optional argument",
					"required": false
				}
			],
			"messages": [
				{
					"role": "user",
					"content": "Hello {{arg1}}"
				},
				{
					"role": "assistant",
					"content": "How can I assist you with {{arg1}}?"
				}
			]
		}`
	s.Run("marshals Prompt correctly", func() {
		data, err := json.Marshal(prompt)
		s.Require().NoError(err, "failed to marshal Prompt to JSON")
		s.JSONEq(jsonData, string(data), "marshaled JSON does not match expected")
	})
	s.Run("unmarshals Prompt correctly", func() {
		var unmarshaled api.Prompt
		err := json.Unmarshal([]byte(jsonData), &unmarshaled)
		s.Require().NoError(err, "failed to unmarshal Prompt from JSON")

		s.Equal(prompt, unmarshaled, "unmarshaled Prompt from JSON")
	})
}

func (s *PromptSerializationSuite) TestPromptTOMLSerialization() {
	prompt := api.Prompt{
		Name:        "test-prompt",
		Title:       "Test Prompt",
		Description: "A test prompt",
		Arguments: []api.PromptArgument{
			{Name: "arg1", Description: "First argument", Required: true},
			{Name: "arg_opt", Description: "Optional argument", Required: false},
		},
		Templates: []api.PromptTemplate{
			{Role: "user", Content: "Hello {{arg1}}"},
			{Role: "assistant", Content: "How can I assist you with {{arg1}}?"},
		},
	}
	tomlData := `
name = "test-prompt"
title = "Test Prompt"
description = "A test prompt"

[[arguments]]
name = "arg1"
description = "First argument"
required = true

[[arguments]]
name = "arg_opt"
description = "Optional argument"
required = false

[[messages]]
role = "user"
content = "Hello {{arg1}}"

[[messages]]
role = "assistant"
content = "How can I assist you with {{arg1}}?"
`
	s.Run("unmarshals Prompt from TOML correctly", func() {
		var unmarshaled api.Prompt
		err := toml.Unmarshal([]byte(tomlData), &unmarshaled)
		s.Require().NoError(err, "failed to unmarshal Prompt from TOML")

		s.Equal(prompt, unmarshaled)
	})
	s.Run("marshals and unmarshals Prompt via round-trip", func() {
		data, err := toml.Marshal(prompt)
		s.Require().NoError(err, "failed to marshal Prompt to TOML")

		var unmarshaled api.Prompt
		err = toml.Unmarshal(data, &unmarshaled)
		s.Require().NoError(err, "failed to unmarshal marshaled TOML")

		s.Equal(prompt, unmarshaled)
	})
}

func (s *PromptSerializationSuite) TestPromptTemplateSerialization() {
	s.Run("serializes template with placeholder", func() {
		template := api.PromptTemplate{
			Role:    "user",
			Content: "Hello {{name}}, how are you?",
		}

		// JSON
		jsonData, err := json.Marshal(template)
		s.Require().NoError(err)
		var jsonTemplate api.PromptTemplate
		err = json.Unmarshal(jsonData, &jsonTemplate)
		s.Require().NoError(err)
		s.Equal(template.Role, jsonTemplate.Role)
		s.Equal(template.Content, jsonTemplate.Content)

		// TOML
		tomlData := `
role = "user"
content = "Hello {{name}}, how are you?"
`
		var tomlTemplate api.PromptTemplate
		err = toml.Unmarshal([]byte(tomlData), &tomlTemplate)
		s.Require().NoError(err)
		s.Equal(template.Role, tomlTemplate.Role)
		s.Equal(template.Content, tomlTemplate.Content)
	})
}

func (s *PromptSerializationSuite) TestPromptMessageSerialization() {
	s.Run("serializes message with content", func() {
		msg := api.PromptMessage{
			Role: "assistant",
			Content: api.PromptContent{
				Type: "text",
				Text: "Hello, World!",
			},
		}

		// JSON
		jsonData, err := json.Marshal(msg)
		s.Require().NoError(err)
		var jsonMsg api.PromptMessage
		err = json.Unmarshal(jsonData, &jsonMsg)
		s.Require().NoError(err)
		s.Equal(msg.Role, jsonMsg.Role)
		s.Equal(msg.Content.Type, jsonMsg.Content.Type)
		s.Equal(msg.Content.Text, jsonMsg.Content.Text)
	})
}

func (s *PromptSerializationSuite) TestPromptContentSerialization() {
	s.Run("serializes text content", func() {
		content := api.PromptContent{
			Type: "text",
			Text: "Sample text content",
		}

		// JSON
		jsonData, err := json.Marshal(content)
		s.Require().NoError(err)
		var jsonContent api.PromptContent
		err = json.Unmarshal(jsonData, &jsonContent)
		s.Require().NoError(err)
		s.Equal(content.Type, jsonContent.Type)
		s.Equal(content.Text, jsonContent.Text)
	})
}

func (s *PromptSerializationSuite) TestPromptWithOptionalFields() {
	s.Run("omits empty optional fields in JSON", func() {
		prompt := api.Prompt{
			Name:        "minimal-prompt",
			Description: "Minimal prompt without optional fields",
		}

		jsonData, err := json.Marshal(prompt)
		s.Require().NoError(err)

		// Verify optional fields are omitted
		var raw map[string]interface{}
		err = json.Unmarshal(jsonData, &raw)
		s.Require().NoError(err)

		s.Contains(raw, "name")
		s.Contains(raw, "description")
		// title is omitempty, should not be present if empty
		_, hasTitle := raw["title"]
		s.False(hasTitle, "empty title should be omitted")
	})

	s.Run("includes optional fields when present", func() {
		prompt := api.Prompt{
			Name:        "full-prompt",
			Title:       "Full Prompt",
			Description: "Prompt with all fields",
			Arguments: []api.PromptArgument{
				{Name: "arg1", Required: true},
			},
		}

		jsonData, err := json.Marshal(prompt)
		s.Require().NoError(err)

		var raw map[string]interface{}
		err = json.Unmarshal(jsonData, &raw)
		s.Require().NoError(err)

		s.Contains(raw, "title")
		s.Equal("Full Prompt", raw["title"])
	})
}

func TestPromptSerialization(t *testing.T) {
	suite.Run(t, new(PromptSerializationSuite))
}
