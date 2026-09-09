package config

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ConfigStateSuite struct {
	suite.Suite
}

func TestConfigState(t *testing.T) {
	suite.Run(t, new(ConfigStateSuite))
}

func portCfg(port string) *Config {
	c := New()
	c.Port.SetForTest(port)
	return c
}

func (s *ConfigStateSuite) TestLoadStore() {
	s.Run("load returns initial config", func() {
		cfg := portCfg("8080")
		state := NewConfigState(cfg)
		s.Equal(cfg, state.Load())
	})

	s.Run("store replaces config", func() {
		cfg1 := portCfg("8080")
		cfg2 := portCfg("9090")
		state := NewConfigState(cfg1)
		state.Store(cfg2)
		s.Equal(cfg2, state.Load())
	})

	s.Run("store ignores nil", func() {
		cfg := portCfg("8080")
		state := NewConfigState(cfg)
		state.Store(nil)
		s.Equal(cfg, state.Load(), "Store(nil) must not clobber the current snapshot")
	})

	s.Run("concurrent load/store is safe", func() {
		state := NewConfigState(portCfg("8080"))
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				state.Store(portCfg("9090"))
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
