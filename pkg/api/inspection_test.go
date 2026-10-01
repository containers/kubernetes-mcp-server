package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ResultsTestSuite struct {
	suite.Suite
}

type resultsTargetProvider struct {
	targets       []string
	targetsErr    error
	defaultTarget string
}

func (p *resultsTargetProvider) IsMultiTarget() bool {
	return len(p.targets) > 1
}

func (p *resultsTargetProvider) GetTargets(context.Context) ([]string, error) {
	return append([]string(nil), p.targets...), p.targetsErr
}

func (p *resultsTargetProvider) GetDefaultTarget() string {
	return p.defaultTarget
}

func (p *resultsTargetProvider) GetTargetParameterName() string {
	return "target"
}

func (s *ResultsTestSuite) TestAny() {
	s.Run("returns true when one target matches", func() {
		provider := &resultsTargetProvider{targets: []string{"a", "b"}}
		results := NewResults(s.T().Context(), provider, func(_ context.Context, target string) (string, error) {
			return target, nil
		})

		matched, err := results.Any(func(value string) bool { return value == "b" })

		s.Require().NoError(err)
		s.True(matched)
	})

	s.Run("returns a target error when no value matches", func() {
		provider := &resultsTargetProvider{targets: []string{"available", "failed"}}
		results := NewResults(s.T().Context(), provider, func(_ context.Context, target string) (string, error) {
			if target == "failed" {
				return "", errors.New("inspection failed")
			}
			return target, nil
		})

		matched, err := results.Any(func(value string) bool { return value == "missing" })

		s.False(matched)
		s.ErrorContains(err, "inspection failed")
	})

	s.Run("discards errors after finding a match", func() {
		provider := &resultsTargetProvider{targets: []string{"matched", "failed"}}
		results := NewResults(s.T().Context(), provider, func(_ context.Context, target string) (bool, error) {
			if target == "failed" {
				return false, errors.New("inspection failed")
			}
			return true, nil
		})

		matched, err := results.Any(func(value bool) bool { return value })

		s.NoError(err)
		s.True(matched)
	})
}

func (s *ResultsTestSuite) TestAll() {
	s.Run("returns true when every target matches", func() {
		provider := &resultsTargetProvider{targets: []string{"a", "b"}}
		results := NewResults(s.T().Context(), provider, func(context.Context, string) (bool, error) {
			return true, nil
		})

		matched, err := results.All(func(value bool) bool { return value })

		s.Require().NoError(err)
		s.True(matched)
	})

	s.Run("returns false without an error when one target does not match", func() {
		provider := &resultsTargetProvider{targets: []string{"matched", "unmatched", "failed"}}
		results := NewResults(s.T().Context(), provider, func(_ context.Context, target string) (bool, error) {
			switch target {
			case "unmatched":
				return false, nil
			case "failed":
				return false, errors.New("inspection failed")
			default:
				return true, nil
			}
		})

		matched, err := results.All(func(value bool) bool { return value })

		s.NoError(err)
		s.False(matched)
	})

	s.Run("returns an error when all successful targets match", func() {
		provider := &resultsTargetProvider{targets: []string{"matched", "failed"}}
		results := NewResults(s.T().Context(), provider, func(_ context.Context, target string) (bool, error) {
			if target == "failed" {
				return false, errors.New("inspection failed")
			}
			return true, nil
		})

		matched, err := results.All(func(value bool) bool { return value })

		s.False(matched)
		s.ErrorContains(err, "inspection failed")
	})
}

func (s *ResultsTestSuite) TestDefaultAndValues() {
	s.Run("default evaluates only the configured default target", func() {
		provider := &resultsTargetProvider{
			targetsErr:    errors.New("targets unavailable"),
			defaultTarget: "primary",
		}
		results := NewResults(s.T().Context(), provider, func(_ context.Context, target string) (string, error) {
			return target, nil
		})

		value, err := results.Default()

		s.Require().NoError(err)
		s.Equal("primary", value)
	})

	s.Run("values returns successes and aggregates target errors", func() {
		provider := &resultsTargetProvider{targets: []string{"a", "b", "failed"}}
		results := NewResults(s.T().Context(), provider, func(_ context.Context, target string) (string, error) {
			if target == "failed" {
				return "", errors.New("inspection failed")
			}
			return target, nil
		})

		values, err := results.Values()

		s.ElementsMatch([]string{"a", "b"}, values)
		s.ErrorContains(err, "inspection failed")
	})
}

func (s *ResultsTestSuite) TestTargetEnumeration() {
	s.Run("returns target enumeration errors", func() {
		provider := &resultsTargetProvider{targetsErr: errors.New("targets unavailable")}
		results := NewResults(s.T().Context(), provider, func(context.Context, string) (bool, error) {
			return true, nil
		})

		matched, err := results.Any(func(value bool) bool { return value })

		s.False(matched)
		s.ErrorContains(err, "failed to fetch targets: targets unavailable")
	})

	s.Run("uses vacuous truth for an empty target set", func() {
		provider := &resultsTargetProvider{}
		results := NewResults(s.T().Context(), provider, func(context.Context, string) (bool, error) {
			return false, nil
		})

		matched, err := results.All(func(value bool) bool { return value })

		s.Require().NoError(err)
		s.True(matched)
	})
}

func TestResults(t *testing.T) {
	suite.Run(t, new(ResultsTestSuite))
}
