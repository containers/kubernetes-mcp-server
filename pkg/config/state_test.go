package config

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
)

type StaticConfigStateSuite struct {
	suite.Suite
}

func TestStaticConfigState(t *testing.T) {
	suite.Run(t, new(StaticConfigStateSuite))
}

func (s *StaticConfigStateSuite) TestLoadStore() {
	s.Run("load returns initial config", func() {
		cfg := &StaticConfig{Port: "8080"}
		state := NewStaticConfigState(cfg)
		s.Equal(cfg, state.Load())
	})

	s.Run("store replaces config", func() {
		cfg1 := &StaticConfig{Port: "8080"}
		cfg2 := &StaticConfig{Port: "9090"}
		state := NewStaticConfigState(cfg1)
		state.Store(cfg2)
		s.Equal(cfg2, state.Load())
	})

	s.Run("store ignores nil", func() {
		cfg := &StaticConfig{Port: "8080"}
		state := NewStaticConfigState(cfg)
		state.Store(nil)
		s.Equal(cfg, state.Load(), "Store(nil) must not clobber the current snapshot")
	})

	s.Run("concurrent load/store is safe", func() {
		state := NewStaticConfigState(&StaticConfig{Port: "8080"})
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				state.Store(&StaticConfig{Port: "9090"})
			}()
			go func() {
				defer wg.Done()
				cfg := state.Load()
				s.NotNil(cfg)
			}()
		}
		wg.Wait()
	})
}
