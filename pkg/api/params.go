package api

import "fmt"

// ErrInvalidInt64Type is returned when a value cannot be converted to int64.
type ErrInvalidInt64Type struct {
	Value interface{}
}

func (e *ErrInvalidInt64Type) Error() string {
	return fmt.Sprintf("expected integer, got %T", e.Value)
}

// ParseInt64 converts an interface{} value (typically from JSON-decoded tool arguments)
// to int64. Handles float64 (JSON numbers), int, and int64 types.
// Returns the converted value and nil error on success, or 0 and an ErrInvalidInt64Type if the type is unsupported.
func ParseInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return 0, &ErrInvalidInt64Type{Value: value}
	}
}

// RequiredString extracts a required string parameter from tool arguments.
// Returns the string value and nil error on success.
// Returns an error if the parameter is missing or not a string.
func RequiredString(params ToolHandlerParams, key string) (string, error) {
	args := params.GetArguments()
	val, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s parameter required", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%s parameter must be a string", key)
	}
	return str, nil
}

// OptionalString extracts an optional string parameter from tool arguments.
// Returns the string value if present and valid, or defaultVal if missing or not a string.
func OptionalString(params ToolHandlerParams, key, defaultVal string) string {
	args := params.GetArguments()
	val, ok := args[key]
	if !ok {
		return defaultVal
	}
	str, ok := val.(string)
	if !ok {
		return defaultVal
	}
	return str
}

// OptionalBool extracts an optional boolean parameter from tool arguments.
// Returns the boolean value if present and valid, or defaultVal if missing or not a boolean.
func OptionalBool(params ToolHandlerParams, key string, defaultVal bool) bool {
	args := params.GetArguments()
	val, ok := args[key]
	if !ok {
		return defaultVal
	}
	b, ok := val.(bool)
	if !ok {
		return defaultVal
	}
	return b
}
