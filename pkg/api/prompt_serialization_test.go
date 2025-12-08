package api

import (
	"encoding/json"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

// PromptSerializationSuite tests serialization of prompt data structures
type PromptSerializationSuite struct {
	suite.Suite
}

func (s *PromptSerializationSuite) TestPromptJSONSerialization() {
	s.Run("marshals and unmarshals Prompt correctly", func() {
		original := Prompt{
			Name:        "test-prompt",
			Title:       "Test Prompt",
			Description: "A test prompt",
			Arguments: []PromptArgument{
				{Name: "arg1", Description: "First argument", Required: true},
			},
			Templates: []PromptTemplate{
				{Role: "user", Content: "Hello {{arg1}}"},
			},
		}

		data, err := json.Marshal(original)
		s.Require().NoError(err, "failed to marshal Prompt to JSON")

		var unmarshaled Prompt
		err = json.Unmarshal(data, &unmarshaled)
		s.Require().NoError(err, "failed to unmarshal Prompt from JSON")

		s.Equal(original.Name, unmarshaled.Name)
		s.Equal(original.Title, unmarshaled.Title)
		s.Equal(original.Description, unmarshaled.Description)
		s.Require().Len(unmarshaled.Arguments, 1)
		s.Equal(original.Arguments[0].Name, unmarshaled.Arguments[0].Name)
		s.Require().Len(unmarshaled.Templates, 1)
		s.Equal(original.Templates[0].Content, unmarshaled.Templates[0].Content)
	})
}

func (s *PromptSerializationSuite) TestPromptYAMLSerialization() {
	s.Run("marshals and unmarshals Prompt correctly", func() {
		original := Prompt{
			Name:        "test-prompt",
			Title:       "Test Prompt",
			Description: "A test prompt",
			Arguments: []PromptArgument{
				{Name: "arg1", Description: "First argument", Required: true},
			},
			Templates: []PromptTemplate{
				{Role: "user", Content: "Hello {{arg1}}"},
			},
		}

		data, err := yaml.Marshal(original)
		s.Require().NoError(err, "failed to marshal Prompt to YAML")

		var unmarshaled Prompt
		err = yaml.Unmarshal(data, &unmarshaled)
		s.Require().NoError(err, "failed to unmarshal Prompt from YAML")

		s.Equal(original.Name, unmarshaled.Name)
		s.Equal(original.Title, unmarshaled.Title)
		s.Equal(original.Description, unmarshaled.Description)
	})
}

func (s *PromptSerializationSuite) TestPromptTOMLSerialization() {
	s.Run("unmarshals Prompt from TOML correctly", func() {
		tomlData := `
name = "test-prompt"
title = "Test Prompt"
description = "A test prompt"

[[arguments]]
name = "arg1"
description = "First argument"
required = true

[[messages]]
role = "user"
content = "Hello {{arg1}}"
`

		var prompt Prompt
		err := toml.Unmarshal([]byte(tomlData), &prompt)
		s.Require().NoError(err, "failed to unmarshal Prompt from TOML")

		s.Equal("test-prompt", prompt.Name)
		s.Equal("Test Prompt", prompt.Title)
		s.Equal("A test prompt", prompt.Description)
		s.Require().Len(prompt.Arguments, 1)
		s.Equal("arg1", prompt.Arguments[0].Name)
		s.Equal("First argument", prompt.Arguments[0].Description)
		s.True(prompt.Arguments[0].Required)
		s.Require().Len(prompt.Templates, 1)
		s.Equal("user", prompt.Templates[0].Role)
		s.Equal("Hello {{arg1}}", prompt.Templates[0].Content)
	})

	s.Run("unmarshals multiple prompts from TOML array", func() {
		tomlData := `
[[prompts]]
name = "prompt1"
description = "First prompt"

[[prompts.messages]]
role = "user"
content = "Message 1"

[[prompts]]
name = "prompt2"
description = "Second prompt"

[[prompts.messages]]
role = "assistant"
content = "Message 2"
`

		var data struct {
			Prompts []Prompt `toml:"prompts"`
		}
		err := toml.Unmarshal([]byte(tomlData), &data)
		s.Require().NoError(err, "failed to unmarshal prompts array from TOML")

		s.Require().Len(data.Prompts, 2)
		s.Equal("prompt1", data.Prompts[0].Name)
		s.Equal("prompt2", data.Prompts[1].Name)
	})
}

func (s *PromptSerializationSuite) TestPromptArgumentSerialization() {
	s.Run("serializes required argument", func() {
		arg := PromptArgument{
			Name:        "test-arg",
			Description: "Test argument",
			Required:    true,
		}

		// JSON
		jsonData, err := json.Marshal(arg)
		s.Require().NoError(err)
		var jsonArg PromptArgument
		err = json.Unmarshal(jsonData, &jsonArg)
		s.Require().NoError(err)
		s.Equal(arg.Name, jsonArg.Name)
		s.True(jsonArg.Required)

		// YAML
		yamlData, err := yaml.Marshal(arg)
		s.Require().NoError(err)
		var yamlArg PromptArgument
		err = yaml.Unmarshal(yamlData, &yamlArg)
		s.Require().NoError(err)
		s.Equal(arg.Name, yamlArg.Name)
		s.True(yamlArg.Required)
	})

	s.Run("serializes optional argument", func() {
		arg := PromptArgument{
			Name:        "optional-arg",
			Description: "Optional argument",
			Required:    false,
		}

		jsonData, err := json.Marshal(arg)
		s.Require().NoError(err)
		var unmarshaled PromptArgument
		err = json.Unmarshal(jsonData, &unmarshaled)
		s.Require().NoError(err)
		s.False(unmarshaled.Required)
	})
}

func (s *PromptSerializationSuite) TestPromptTemplateSerialization() {
	s.Run("serializes template with placeholder", func() {
		template := PromptTemplate{
			Role:    "user",
			Content: "Hello {{name}}, how are you?",
		}

		// JSON
		jsonData, err := json.Marshal(template)
		s.Require().NoError(err)
		var jsonTemplate PromptTemplate
		err = json.Unmarshal(jsonData, &jsonTemplate)
		s.Require().NoError(err)
		s.Equal(template.Role, jsonTemplate.Role)
		s.Equal(template.Content, jsonTemplate.Content)

		// TOML
		tomlData := `
role = "user"
content = "Hello {{name}}, how are you?"
`
		var tomlTemplate PromptTemplate
		err = toml.Unmarshal([]byte(tomlData), &tomlTemplate)
		s.Require().NoError(err)
		s.Equal(template.Role, tomlTemplate.Role)
		s.Equal(template.Content, tomlTemplate.Content)
	})
}

func (s *PromptSerializationSuite) TestPromptMessageSerialization() {
	s.Run("serializes message with content", func() {
		msg := PromptMessage{
			Role: "assistant",
			Content: PromptContent{
				Type: "text",
				Text: "Hello, World!",
			},
		}

		// JSON
		jsonData, err := json.Marshal(msg)
		s.Require().NoError(err)
		var jsonMsg PromptMessage
		err = json.Unmarshal(jsonData, &jsonMsg)
		s.Require().NoError(err)
		s.Equal(msg.Role, jsonMsg.Role)
		s.Equal(msg.Content.Type, jsonMsg.Content.Type)
		s.Equal(msg.Content.Text, jsonMsg.Content.Text)

		// YAML
		yamlData, err := yaml.Marshal(msg)
		s.Require().NoError(err)
		var yamlMsg PromptMessage
		err = yaml.Unmarshal(yamlData, &yamlMsg)
		s.Require().NoError(err)
		s.Equal(msg.Role, yamlMsg.Role)
		s.Equal(msg.Content.Text, yamlMsg.Content.Text)
	})
}

func (s *PromptSerializationSuite) TestPromptContentSerialization() {
	s.Run("serializes text content", func() {
		content := PromptContent{
			Type: "text",
			Text: "Sample text content",
		}

		// JSON
		jsonData, err := json.Marshal(content)
		s.Require().NoError(err)
		var jsonContent PromptContent
		err = json.Unmarshal(jsonData, &jsonContent)
		s.Require().NoError(err)
		s.Equal(content.Type, jsonContent.Type)
		s.Equal(content.Text, jsonContent.Text)

		// YAML
		yamlData, err := yaml.Marshal(content)
		s.Require().NoError(err)
		var yamlContent PromptContent
		err = yaml.Unmarshal(yamlData, &yamlContent)
		s.Require().NoError(err)
		s.Equal(content.Type, yamlContent.Type)
		s.Equal(content.Text, yamlContent.Text)
	})
}

func (s *PromptSerializationSuite) TestPromptWithOptionalFields() {
	s.Run("omits empty optional fields in JSON", func() {
		prompt := Prompt{
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
		prompt := Prompt{
			Name:        "full-prompt",
			Title:       "Full Prompt",
			Description: "Prompt with all fields",
			Arguments: []PromptArgument{
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
