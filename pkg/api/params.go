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
//
// Deprecated: this helper silently returns defaultVal when the argument is
// present but of the wrong type, masking client errors. New code should use
// WrapParams and the Params.OptionalString method, which records a sticky
// type-mismatch error exposed via Params.Err.
func OptionalString(params ToolHandlerParams, key, defaultVal string) string {
	str, err := optionalString(params, key, defaultVal)
	if err != nil {
		return defaultVal
	}
	return str
}

// OptionalBool extracts an optional boolean parameter from tool arguments.
// Returns the boolean value if present and valid, or defaultVal if missing or not a boolean.
//
// Deprecated: this helper silently returns defaultVal when the argument is
// present but of the wrong type, masking client errors. New code should use
// WrapParams and the Params.OptionalBool method, which records a sticky
// type-mismatch error exposed via Params.Err.
func OptionalBool(params ToolHandlerParams, key string, defaultVal bool) bool {
	b, err := optionalBool(params, key, defaultVal)
	if err != nil {
		return defaultVal
	}
	return b
}

// optionalString is the type-strict variant that powers Params.OptionalString.
// Missing key returns defaultVal with no error; a present-but-wrong-type value
// returns an error.
func optionalString(params ToolHandlerParams, key, defaultVal string) (string, error) {
	val, ok := params.GetArguments()[key]
	if !ok {
		return defaultVal, nil
	}
	str, ok := val.(string)
	if !ok {
		return defaultVal, fmt.Errorf("%s parameter must be a string", key)
	}
	return str, nil
}

// optionalBool is the type-strict variant that powers Params.OptionalBool.
// Missing key returns defaultVal with no error; a present-but-wrong-type value
// returns an error.
func optionalBool(params ToolHandlerParams, key string, defaultVal bool) (bool, error) {
	val, ok := params.GetArguments()[key]
	if !ok {
		return defaultVal, nil
	}
	b, ok := val.(bool)
	if !ok {
		return defaultVal, fmt.Errorf("%s parameter must be a boolean", key)
	}
	return b, nil
}

// Params wraps ToolHandlerParams with sticky-error parameter extraction, so a
// handler can extract several arguments and check for type mismatches once at
// the end rather than after each call. Once an extraction fails, subsequent
// extractions return their default/zero value and do not overwrite the first
// error (matching the idiom used by bufio.Scanner and sql.Rows).
//
// Unlike the package-level OptionalString / OptionalBool helpers, the Params
// methods surface type mismatches as errors instead of silently falling back
// to the default: the tool InputSchema advertises the expected type, so a
// present-but-wrong-type value indicates a malformed client call that the
// caller should see.
//
// Typical usage:
//
//	p := api.WrapParams(params)
//	ns := p.OptionalString("namespace", "")
//	name := p.RequiredString("name")
//	tail := p.OptionalInt64("tail", 0)
//	if err := p.Err(); err != nil {
//	    return api.NewToolCallResult("", fmt.Errorf("failed to X: %w", err)), nil
//	}
type Params struct {
	ToolHandlerParams
	err error
}

// WrapParams returns a sticky-error wrapper around the given ToolHandlerParams.
func WrapParams(params ToolHandlerParams) *Params {
	return &Params{ToolHandlerParams: params}
}

// Err returns the first type-mismatch error encountered by any extraction
// call, or nil if all extractions succeeded.
func (p *Params) Err() error { return p.err }

// RequiredString is the sticky-error variant of the package-level RequiredString.
func (p *Params) RequiredString(key string) string {
	if p.err != nil {
		return ""
	}
	v, err := RequiredString(p.ToolHandlerParams, key)
	if err != nil {
		p.err = err
	}
	return v
}

// OptionalString extracts an optional string. Missing key returns defaultVal
// with no sticky error; a present-but-wrong-type value records a sticky error
// and returns defaultVal.
func (p *Params) OptionalString(key, defaultVal string) string {
	if p.err != nil {
		return defaultVal
	}
	v, err := optionalString(p.ToolHandlerParams, key, defaultVal)
	if err != nil {
		p.err = err
		return defaultVal
	}
	return v
}

// OptionalBool extracts an optional bool. Missing key returns defaultVal with
// no sticky error; a present-but-wrong-type value records a sticky error and
// returns defaultVal.
func (p *Params) OptionalBool(key string, defaultVal bool) bool {
	if p.err != nil {
		return defaultVal
	}
	v, err := optionalBool(p.ToolHandlerParams, key, defaultVal)
	if err != nil {
		p.err = err
		return defaultVal
	}
	return v
}

// OptionalInt64 extracts an optional int64 parameter. Missing key returns
// defaultVal with no sticky error; a present-but-wrong-type value records a
// sticky error and returns defaultVal.
func (p *Params) OptionalInt64(key string, defaultVal int64) int64 {
	if p.err != nil {
		return defaultVal
	}
	val, ok := p.GetArguments()[key]
	if !ok || val == nil {
		return defaultVal
	}
	v, err := ParseInt64(val)
	if err != nil {
		p.err = fmt.Errorf("%s parameter must be an integer: %w", key, err)
		return defaultVal
	}
	return v
}
