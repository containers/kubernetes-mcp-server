package api

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/suite"
)

type ResultBudgetSuite struct {
	suite.Suite
}

func TestResultBudget(t *testing.T) {
	suite.Run(t, new(ResultBudgetSuite))
}

func (s *ResultBudgetSuite) TestWriteCutsAndAppendsTheNoticeOnce() {
	notice := ResultTruncationNotice
	b := NewBuilderWithBudget(4)
	s.True(b.WriteString("abcdef"))
	s.Equal("abcd"+notice, b.String())
	s.Equal(4+len(notice), b.Len())
	s.True(b.Truncated())
	s.True(b.WriteString("z"))
	s.Equal("abcd"+notice, b.String())
}

func (s *ResultBudgetSuite) TestExactFitDoesNotTruncate() {
	b := NewBuilderWithBudget(4)
	s.False(b.WriteString("abcd"))
	s.Equal("abcd", b.String())
	s.False(b.Truncated())
	s.True(b.WriteString("e"))
	s.Equal("abcd"+ResultTruncationNotice, b.String())
	s.True(b.WriteString("f"))
	s.Equal("abcd"+ResultTruncationNotice, b.String())
}

func (s *ResultBudgetSuite) TestCutDoesNotSplitARune() {
	b := NewBuilderWithBudget(4)
	s.True(b.WriteString("aaaé"))
	s.Equal("aaa"+ResultTruncationNotice, b.String())
	s.True(utf8.ValidString(b.String()))
}

func (s *ResultBudgetSuite) TestNoRoomWritesOnlyTheNotice() {
	b := NewBuilderWithBudget(4)
	s.False(b.WriteString("abcd"))
	s.True(b.WriteString("more"))
	s.Equal("abcd"+ResultTruncationNotice, b.String())
}

func (s *ResultBudgetSuite) TestDisabledBudgetWritesEverything() {
	b := NewBuilderWithBudget(0)
	s.False(b.WriteString("abcdef"))
	s.Equal("abcdef", b.String())
	s.False(b.Truncated())

	b = NewBuilderWithBudget(-1)
	s.False(b.WriteString("abcdef"))
	s.Equal("abcdef", b.String())
}

func (s *ResultBudgetSuite) TestLimitStringUsesTheContextCap() {
	ctx := WithResultBudget(context.Background(), 4)
	s.Equal(int64(4), ResultLimit(ctx))
	s.Equal("abcd"+ResultTruncationNotice, LimitString(ctx, "abcdef"))
	s.Equal("abcdef", LimitString(context.Background(), "abcdef"))
	//lint:ignore SA1012 we're testing the path where WithResultBudget subs an ephemeral context when it receives nil
	s.Equal(int64(3), ResultLimit(WithResultBudget(nil, 3))) //nolint:staticcheck // we're testing the path where WithResultBudget subs an ephemeral context when it receives nil
	s.Equal(int64(0), ResultLimit(WithResultBudget(context.Background(), 0)))
}

func (s *ResultBudgetSuite) TestScratchCapsTheNestedCollect() {
	parent := NewBuilderWithBudget(20)
	s.Equal(int64(20), parent.Remaining())

	scratch := parent.Scratch(4)
	s.Equal(int64(16), scratch.Remaining())
	s.True(scratch.WriteString(strings.Repeat("a", 100)))
	s.True(scratch.Truncated())
	s.Equal(int64(20), parent.Remaining(), "a scratch does not charge the parent")

	s.Run("no room left is already truncated", func() {
		full := NewBuilderWithBudget(4)
		s.False(full.WriteString("abcd"))
		child := full.Scratch(1)
		s.True(child.Truncated())
		s.Empty(child.String())
		s.True(child.WriteString("more"))
		s.Empty(child.String())
	})

	s.Run("uncapped parent yields an uncapped scratch", func() {
		open := NewBuilderWithBudget(0)
		s.Equal(int64(-1), open.Remaining())
		child := open.Scratch(8)
		s.False(child.WriteString(strings.Repeat("b", 30)))
		s.Equal(30, child.Len())
		s.False(child.Truncated())
	})
}

func (s *ResultBudgetSuite) TestFitSectionsStopsAfterTheFirstCut() {
	notice := ResultTruncationNotice
	s.Equal([]string{"abcd", ""}, FitSections(4, "abcd", ""))
	s.Equal([]string{"abcd", notice}, FitSections(4, "abcd", "e"))
	s.Equal([]string{"ab", "cd", notice}, FitSections(4, "ab", "cd", "e"))
	s.Equal([]string{"abcd" + notice, ""}, FitSections(4, "abcdef", "zz"))
	s.Equal([]string{notice}, FitSections(0, "abc"))
}
