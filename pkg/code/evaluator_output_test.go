package code

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// EvaluatorOutputSuite tests output formatting and the valueToString function.
type EvaluatorOutputSuite struct {
	BaseEvaluatorSuite
	evaluator *Evaluator
}

func (s *EvaluatorOutputSuite) SetupTest() {
	s.BaseEvaluatorSuite.SetupTest()
	var err error
	s.evaluator, err = NewEvaluator(context.Background(), s.kubernetes)
	s.Require().NoError(err)
}

func (s *EvaluatorOutputSuite) TestStringOutput() {
	s.Run("simple string is returned as-is", func() {
		result, err := s.evaluator.Evaluate(`"hello world"`, time.Second)
		s.NoError(err)
		s.Equal("hello world", result)
	})

	s.Run("empty string returns empty", func() {
		result, err := s.evaluator.Evaluate(`""`, time.Second)
		s.NoError(err)
		s.Equal("", result)
	})

	s.Run("string with special characters is preserved", func() {
		result, err := s.evaluator.Evaluate(`"hello\nworld\ttab"`, time.Second)
		s.NoError(err)
		s.Equal("hello\nworld\ttab", result)
	})

	s.Run("unicode string is preserved", func() {
		result, err := s.evaluator.Evaluate(`"日本語 🎉"`, time.Second)
		s.NoError(err)
		s.Equal("日本語 🎉", result)
	})
}

func (s *EvaluatorOutputSuite) TestNumericOutput() {
	s.Run("positive integer", func() {
		result, err := s.evaluator.Evaluate(`42`, time.Second)
		s.NoError(err)
		s.Equal("42", result)
	})

	s.Run("negative integer", func() {
		result, err := s.evaluator.Evaluate(`-123`, time.Second)
		s.NoError(err)
		s.Equal("-123", result)
	})

	s.Run("float with decimals", func() {
		result, err := s.evaluator.Evaluate(`3.14159`, time.Second)
		s.NoError(err)
		s.Equal("3.14159", result)
	})

	s.Run("large number (MAX_SAFE_INTEGER)", func() {
		result, err := s.evaluator.Evaluate(`9007199254740991`, time.Second)
		s.NoError(err)
		s.Equal("9007199254740991", result)
	})

	s.Run("arithmetic expression result", func() {
		result, err := s.evaluator.Evaluate(`2 + 2`, time.Second)
		s.NoError(err)
		s.Equal("4", result)
	})
}

func (s *EvaluatorOutputSuite) TestBooleanOutput() {
	s.Run("true is returned as string", func() {
		result, err := s.evaluator.Evaluate(`true`, time.Second)
		s.NoError(err)
		s.Equal("true", result)
	})

	s.Run("false is returned as string", func() {
		result, err := s.evaluator.Evaluate(`false`, time.Second)
		s.NoError(err)
		s.Equal("false", result)
	})
}

func (s *EvaluatorOutputSuite) TestNullAndUndefinedOutput() {
	s.Run("null returns empty string", func() {
		result, err := s.evaluator.Evaluate(`null`, time.Second)
		s.NoError(err)
		s.Equal("", result)
	})

	s.Run("undefined returns empty string", func() {
		result, err := s.evaluator.Evaluate(`undefined`, time.Second)
		s.NoError(err)
		s.Equal("", result)
	})
}

func (s *EvaluatorOutputSuite) TestJSONOutput() {
	s.Run("output is compact without indentation", func() {
		result, err := s.evaluator.Evaluate(`({a: 1, b: "two"})`, time.Second)
		s.NoError(err)
		s.Equal(`{"a":1,"b":"two"}`, result)
		s.NotContains(result, "\n", "JSON should have no newlines")
		s.NotContains(result, "  ", "JSON should have no indentation")
	})

	s.Run("nested objects are compact", func() {
		result, err := s.evaluator.Evaluate(`({outer: {inner: {deep: "value"}}})`, time.Second)
		s.NoError(err)
		s.Equal(`{"outer":{"inner":{"deep":"value"}}}`, result)
		s.NotContains(result, "\n")
	})

	s.Run("arrays are compact", func() {
		result, err := s.evaluator.Evaluate(`[1, 2, 3]`, time.Second)
		s.NoError(err)
		s.Equal(`[1,2,3]`, result)
	})

	s.Run("mixed array with different types", func() {
		result, err := s.evaluator.Evaluate(`[1, "two", true, null]`, time.Second)
		s.NoError(err)
		s.Equal(`[1,"two",true,null]`, result)
	})

	s.Run("array of objects", func() {
		result, err := s.evaluator.Evaluate(`[{a: 1}, {b: 2}]`, time.Second)
		s.NoError(err)
		s.Equal(`[{"a":1},{"b":2}]`, result)
	})

	s.Run("empty object", func() {
		result, err := s.evaluator.Evaluate(`({})`, time.Second)
		s.NoError(err)
		s.Equal(`{}`, result)
	})

	s.Run("empty array", func() {
		result, err := s.evaluator.Evaluate(`[]`, time.Second)
		s.NoError(err)
		s.Equal(`[]`, result)
	})
}

func (s *EvaluatorOutputSuite) TestExpressionAndFunctionOutput() {
	s.Run("function return value is captured", func() {
		result, err := s.evaluator.Evaluate(`(function() { return "from function"; })()`, time.Second)
		s.NoError(err)
		s.Equal("from function", result)
	})

	s.Run("last expression in script is returned", func() {
		result, err := s.evaluator.Evaluate(`
			const x = 1;
			const y = 2;
			x + y;
		`, time.Second)
		s.NoError(err)
		s.Equal("3", result)
	})

	s.Run("Date toISOString returns string", func() {
		result, err := s.evaluator.Evaluate(`new Date(0).toISOString()`, time.Second)
		s.NoError(err)
		s.Equal("1970-01-01T00:00:00.000Z", result)
	})
}

func TestEvaluatorOutput(t *testing.T) {
	suite.Run(t, new(EvaluatorOutputSuite))
}
