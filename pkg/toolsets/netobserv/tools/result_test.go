package tools

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ResultSuite struct {
	suite.Suite
}

func (s *ResultSuite) TestJSONAPIResult() {
	s.Run("returns structured content for valid JSON", func() {
		result, err := jsonAPIResult(`{"result":[],"stats":{"flows":1}}`, nil)
		s.Require().NoError(err)
		s.Nil(result.Error)
		s.NotNil(result.StructuredContent)
		structured, ok := result.StructuredContent.(map[string]any)
		s.Require().True(ok)
		s.Contains(structured, "result")
		s.Contains(result.Content, "flows")
	})

	s.Run("falls back to text for non-JSON", func() {
		result, err := jsonAPIResult("not-json", nil)
		s.Require().NoError(err)
		s.Nil(result.StructuredContent)
		s.Equal("not-json", result.Content)
	})

	s.Run("wraps API errors", func() {
		result, err := jsonAPIResult("", wrapAPIError("list flow records", errors.New("upstream failure")))
		s.Require().NoError(err)
		s.Error(result.Error)
		s.Contains(result.Error.Error(), "list flow records")
	})
}

func TestResultSuite(t *testing.T) {
	suite.Run(t, new(ResultSuite))
}
