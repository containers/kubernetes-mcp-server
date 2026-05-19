package netobserv

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type QuerySuite struct {
	suite.Suite
}

func (s *QuerySuite) TestArgumentsToValues_match() {
	s.Run("expands match to prometheus match[]", func() {
		values := ArgumentsToValues(map[string]any{
			"match": "alertname=NetObserv_*",
			"type":  "alert",
		})
		s.Equal([]string{"{alertname=NetObserv_*}"}, values["match[]"])
		s.Equal("alert", values.Get("type"))
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
