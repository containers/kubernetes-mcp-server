package api

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ParamsSuite struct {
	suite.Suite
}

func TestParamsSuite(t *testing.T) {
	suite.Run(t, new(ParamsSuite))
}

func (s *ParamsSuite) TestParseInt64() {
	s.Run("float64 value is converted to int64", func() {
		result, err := ParseInt64(float64(42.0))
		s.NoError(err)
		s.Equal(int64(42), result)
	})

	s.Run("float64 with decimal truncates to int64", func() {
		result, err := ParseInt64(float64(42.9))
		s.NoError(err)
		s.Equal(int64(42), result)
	})

	s.Run("int value is converted to int64", func() {
		result, err := ParseInt64(int(100))
		s.NoError(err)
		s.Equal(int64(100), result)
	})

	s.Run("int64 value is returned as-is", func() {
		result, err := ParseInt64(int64(999))
		s.NoError(err)
		s.Equal(int64(999), result)
	})

	s.Run("negative float64 value is converted correctly", func() {
		result, err := ParseInt64(float64(-10.0))
		s.NoError(err)
		s.Equal(int64(-10), result)
	})

	s.Run("negative int value is converted correctly", func() {
		result, err := ParseInt64(int(-5))
		s.NoError(err)
		s.Equal(int64(-5), result)
	})

	s.Run("zero value is handled correctly", func() {
		result, err := ParseInt64(float64(0))
		s.NoError(err)
		s.Equal(int64(0), result)
	})

	s.Run("string value returns error", func() {
		result, err := ParseInt64("not a number")
		s.Error(err)
		s.Equal(int64(0), result)
		s.Contains(err.Error(), "string")
	})

	s.Run("nil value returns error", func() {
		result, err := ParseInt64(nil)
		s.Error(err)
		s.Equal(int64(0), result)
	})

	s.Run("bool value returns error", func() {
		result, err := ParseInt64(true)
		s.Error(err)
		s.Equal(int64(0), result)
		s.Contains(err.Error(), "bool")
	})

	s.Run("slice value returns error", func() {
		result, err := ParseInt64([]int{1, 2, 3})
		s.Error(err)
		s.Equal(int64(0), result)
	})

	s.Run("map value returns error", func() {
		result, err := ParseInt64(map[string]int{"a": 1})
		s.Error(err)
		s.Equal(int64(0), result)
	})
}

func (s *ParamsSuite) TestErrInvalidInt64Type() {
	s.Run("error includes type information", func() {
		err := &ErrInvalidInt64Type{Value: "test"}
		s.Contains(err.Error(), "string")
	})

	s.Run("error can be type asserted", func() {
		_, err := ParseInt64("invalid")
		var typeErr *ErrInvalidInt64Type
		s.ErrorAs(err, &typeErr)
		s.Equal("invalid", typeErr.Value)
	})
}

// mockToolCallRequest implements ToolCallRequest for testing
type mockToolCallRequest struct {
	args map[string]any
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.args
}

func (s *ParamsSuite) TestRequiredString() {
	s.Run("returns string value when present", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{"name": "test-value"}}}
		result, err := RequiredString(params, "name")
		s.NoError(err)
		s.Equal("test-value", result)
	})

	s.Run("returns error when key is missing", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{}}}
		result, err := RequiredString(params, "name")
		s.Error(err)
		s.Equal("", result)
		s.Contains(err.Error(), "name parameter required")
	})

	s.Run("returns error when value is not a string", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{"name": 123}}}
		result, err := RequiredString(params, "name")
		s.Error(err)
		s.Equal("", result)
		s.Contains(err.Error(), "name parameter must be a string")
	})

	s.Run("returns empty string when value is empty string", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{"name": ""}}}
		result, err := RequiredString(params, "name")
		s.NoError(err)
		s.Equal("", result)
	})
}

func (s *ParamsSuite) TestOptionalString() {
	s.Run("returns string value when present", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{"name": "test-value"}}}
		result := OptionalString(params, "name", "default")
		s.Equal("test-value", result)
	})

	s.Run("returns default when key is missing", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{}}}
		result := OptionalString(params, "name", "default-value")
		s.Equal("default-value", result)
	})

	s.Run("returns default when value is not a string", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{"name": 123}}}
		result := OptionalString(params, "name", "fallback")
		s.Equal("fallback", result)
	})

	s.Run("returns empty string when value is empty string", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{"name": ""}}}
		result := OptionalString(params, "name", "default")
		s.Equal("", result)
	})

	s.Run("returns empty string when default is empty and key is missing", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{}}}
		result := OptionalString(params, "name", "")
		s.Equal("", result)
	})
}

func (s *ParamsSuite) TestOptionalBool() {
	s.Run("returns true when value is true", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{"enabled": true}}}
		result := OptionalBool(params, "enabled", false)
		s.True(result)
	})

	s.Run("returns false when value is false", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{"enabled": false}}}
		result := OptionalBool(params, "enabled", true)
		s.False(result)
	})

	s.Run("returns default when key is missing", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{}}}
		result := OptionalBool(params, "enabled", true)
		s.True(result)
	})

	s.Run("returns default when value is not a bool", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{"enabled": "true"}}}
		result := OptionalBool(params, "enabled", true)
		s.True(result)
	})

	s.Run("returns false default when key is missing", func() {
		params := ToolHandlerParams{ToolCallRequest: &mockToolCallRequest{args: map[string]any{}}}
		result := OptionalBool(params, "enabled", false)
		s.False(result)
	})
}
