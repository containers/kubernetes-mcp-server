package tekton

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type TaskRunLogSuite struct {
	suite.Suite
}

func (s *TaskRunLogSuite) TestReadLogTail() {
	s.Run("retains newest bytes when truncated", func() {
		actual, truncated, err := readLogTail(strings.NewReader("oldest-newest"), 6)
		s.Require().NoError(err)
		s.Equal("newest", actual)
		s.True(truncated)
	})

	s.Run("preserves short logs", func() {
		actual, truncated, err := readLogTail(strings.NewReader("complete"), 8)
		s.Require().NoError(err)
		s.Equal("complete", actual)
		s.False(truncated)
	})
}

func (s *TaskRunLogSuite) TestTruncateUTF8BytesRetainsNewestValidText() {
	actual, truncated := truncateUTF8Bytes("old界new", 5)
	s.Equal("new", actual)
	s.True(truncated)
}

func TestTaskRunLog(t *testing.T) {
	suite.Run(t, new(TaskRunLogSuite))
}
