package netobserv

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type QuerySuite struct {
	suite.Suite
}

func (s *QuerySuite) TestPrepareQueryArguments_timeRange() {
	s.Run("converts timeRange to startTime and endTime", func() {
		before := time.Now().Unix()
		prepared := PrepareQueryArguments(map[string]any{
			"namespace": "default",
			"timeRange": 300,
		})
		after := time.Now().Unix()

		s.Equal("default", prepared["namespace"])
		_, hasTimeRange := prepared["timeRange"]
		s.False(hasTimeRange)

		endTime, ok := int64Arg(prepared["endTime"])
		s.True(ok)
		startTime, ok := int64Arg(prepared["startTime"])
		s.True(ok)
		s.GreaterOrEqual(endTime, before)
		s.LessOrEqual(endTime, after)
		s.Equal(int64(300), endTime-startTime)
	})

	s.Run("preserves explicit startTime and sets endTime to now", func() {
		before := time.Now().Unix()
		prepared := PrepareQueryArguments(map[string]any{
			"startTime": int64(1_700_000_000),
			"timeRange": 60,
		})
		after := time.Now().Unix()

		s.Equal(int64(1_700_000_000), prepared["startTime"])
		endTime, ok := int64Arg(prepared["endTime"])
		s.True(ok)
		s.GreaterOrEqual(endTime, before)
		s.LessOrEqual(endTime, after)
	})

	s.Run("uses timeRange with explicit endTime when startTime omitted", func() {
		prepared := PrepareQueryArguments(map[string]any{
			"endTime":   int64(1_700_000_300),
			"timeRange": 120,
		})

		s.Equal(int64(1_700_000_300), prepared["endTime"])
		s.Equal(int64(1_700_000_180), prepared["startTime"])
	})
}

func (s *QuerySuite) TestArgumentsToValues_skips_empty() {
	s.Run("omits empty strings", func() {
		values := ArgumentsToValues(map[string]any{
			"namespace": "",
			"limit":     10,
		})
		s.Empty(values.Get("namespace"))
		s.Equal("10", values.Get("limit"))
	})
}

func TestQuery(t *testing.T) {
	suite.Run(t, new(QuerySuite))
}
