package tools

import "github.com/google/jsonschema-go/jsonschema"

// MustSchema returns the JSON Schema for type T, panicking on error.
func MustSchema[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	return s
}
