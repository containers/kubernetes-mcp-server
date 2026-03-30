package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type DurationSuite struct {
	suite.Suite
}

func (s *DurationSuite) TestUnmarshalText() {
	s.Run("parses seconds", func() {
		var d Duration
		err := d.UnmarshalText([]byte("30s"))
		s.Require().NoError(err)
		s.Equal(30*time.Second, d.Duration())
	})

	s.Run("parses minutes", func() {
		var d Duration
		err := d.UnmarshalText([]byte("5m"))
		s.Require().NoError(err)
		s.Equal(5*time.Minute, d.Duration())
	})

	s.Run("parses hours", func() {
		var d Duration
		err := d.UnmarshalText([]byte("2h"))
		s.Require().NoError(err)
		s.Equal(2*time.Hour, d.Duration())
	})

	s.Run("parses compound duration", func() {
		var d Duration
		err := d.UnmarshalText([]byte("1h30m"))
		s.Require().NoError(err)
		s.Equal(90*time.Minute, d.Duration())
	})

	s.Run("parses milliseconds", func() {
		var d Duration
		err := d.UnmarshalText([]byte("500ms"))
		s.Require().NoError(err)
		s.Equal(500*time.Millisecond, d.Duration())
	})

	s.Run("returns error for invalid duration", func() {
		var d Duration
		err := d.UnmarshalText([]byte("invalid"))
		s.Error(err)
		s.Contains(err.Error(), "invalid duration")
	})

	s.Run("returns error for empty string", func() {
		var d Duration
		err := d.UnmarshalText([]byte(""))
		s.Error(err)
	})
}

func (s *DurationSuite) TestMarshalText() {
	s.Run("marshals seconds", func() {
		d := Duration(30 * time.Second)
		text, err := d.MarshalText()
		s.Require().NoError(err)
		s.Equal("30s", string(text))
	})

	s.Run("marshals minutes", func() {
		d := Duration(5 * time.Minute)
		text, err := d.MarshalText()
		s.Require().NoError(err)
		s.Equal("5m0s", string(text))
	})

	s.Run("marshals compound duration", func() {
		d := Duration(90 * time.Minute)
		text, err := d.MarshalText()
		s.Require().NoError(err)
		s.Equal("1h30m0s", string(text))
	})

	s.Run("marshals zero duration", func() {
		d := Duration(0)
		text, err := d.MarshalText()
		s.Require().NoError(err)
		s.Equal("0s", string(text))
	})
}

func (s *DurationSuite) TestDuration() {
	s.Run("returns underlying time.Duration", func() {
		d := Duration(42 * time.Second)
		s.Equal(42*time.Second, d.Duration())
	})
}

func TestDuration(t *testing.T) {
	suite.Run(t, new(DurationSuite))
}
