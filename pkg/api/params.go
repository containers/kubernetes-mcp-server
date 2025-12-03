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
