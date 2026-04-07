package tekton

import (
	"testing"

	"github.com/stretchr/testify/suite"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

type ParamsSuite struct {
	suite.Suite
}

func TestParams(t *testing.T) {
	suite.Run(t, new(ParamsSuite))
}

func (s *ParamsSuite) TestParseParams() {
	s.Run("string parameter", func() {
		input := map[string]interface{}{
			"message": "hello world",
		}

		result, err := parseParams(input)

		s.NoError(err)
		s.Require().Len(result, 1)
		s.Equal("message", result[0].Name)
		s.Equal(tektonv1.ParamTypeString, result[0].Value.Type)
		s.Equal("hello world", result[0].Value.StringVal)
	})

	s.Run("array parameter", func() {
		input := map[string]interface{}{
			"items": []interface{}{"item1", "item2", "item3"},
		}

		result, err := parseParams(input)

		s.NoError(err)
		s.Require().Len(result, 1)
		s.Equal("items", result[0].Name)
		s.Equal(tektonv1.ParamTypeArray, result[0].Value.Type)
		s.Equal([]string{"item1", "item2", "item3"}, result[0].Value.ArrayVal)
	})

	s.Run("object parameter", func() {
		input := map[string]interface{}{
			"config": map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		}

		result, err := parseParams(input)

		s.NoError(err)
		s.Require().Len(result, 1)
		s.Equal("config", result[0].Name)
		s.Equal(tektonv1.ParamTypeObject, result[0].Value.Type)
		s.Equal(map[string]string{"key1": "value1", "key2": "value2"}, result[0].Value.ObjectVal)
	})

	s.Run("mixed parameter types", func() {
		input := map[string]interface{}{
			"stringParam": "test",
			"arrayParam":  []interface{}{"a", "b"},
			"objectParam": map[string]interface{}{"x": "y"},
		}

		result, err := parseParams(input)

		s.NoError(err)
		s.Len(result, 3)

		paramsByName := make(map[string]tektonv1.Param)
		for _, p := range result {
			paramsByName[p.Name] = p
		}

		s.Equal(tektonv1.ParamTypeString, paramsByName["stringParam"].Value.Type)
		s.Equal("test", paramsByName["stringParam"].Value.StringVal)

		s.Equal(tektonv1.ParamTypeArray, paramsByName["arrayParam"].Value.Type)
		s.Equal([]string{"a", "b"}, paramsByName["arrayParam"].Value.ArrayVal)

		s.Equal(tektonv1.ParamTypeObject, paramsByName["objectParam"].Value.Type)
		s.Equal(map[string]string{"x": "y"}, paramsByName["objectParam"].Value.ObjectVal)
	})

	s.Run("array with non-string element", func() {
		input := map[string]interface{}{
			"badArray": []interface{}{"string", 123},
		}

		result, err := parseParams(input)

		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "array element 1 must be a string")
	})

	s.Run("unsupported type", func() {
		input := map[string]interface{}{
			"number": 42,
		}

		result, err := parseParams(input)

		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "unsupported value type")
	})
}
