package code

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type EvaluatorSecuritySuite struct {
	BaseEvaluatorSuite
	*Evaluator
}

func (s *EvaluatorSecuritySuite) SetupTest() {
	s.BaseEvaluatorSuite.SetupTest()
	var err error
	s.Evaluator, err = NewEvaluator(context.Background(), s.kubernetes)
	s.Require().NoError(err)
}

func (s *EvaluatorSecuritySuite) TestNoNetworkAccess() {
	s.Run("fetch is not defined", func() {
		_, err := s.Evaluate("fetch('http://example.com')", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})

	s.Run("XMLHttpRequest is not defined", func() {
		_, err := s.Evaluate("new XMLHttpRequest()", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})

	s.Run("WebSocket is not defined", func() {
		_, err := s.Evaluate("new WebSocket('ws://example.com')", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})
}

func (s *EvaluatorSecuritySuite) TestNoFileSystemAccess() {
	s.Run("require is not defined", func() {
		_, err := s.Evaluate("require('fs')", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})

	s.Run("import is not available for modules", func() {
		// Dynamic import should fail
		_, err := s.Evaluate("import('fs')", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "Unexpected reserved word")
	})
}

func (s *EvaluatorSecuritySuite) TestNoProcessAccess() {
	s.Run("process is not defined", func() {
		_, err := s.Evaluate("process.env", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})

	s.Run("process.exit is not available", func() {
		_, err := s.Evaluate("process.exit(1)", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})

	s.Run("process.cwd is not available", func() {
		_, err := s.Evaluate("process.cwd()", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})
}

func (s *EvaluatorSecuritySuite) TestNoGlobalObjectAccess() {
	s.Run("global is not defined", func() {
		_, err := s.Evaluate("global.process", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})

	s.Run("globalThis has no process", func() {
		result, err := s.Evaluate("typeof globalThis.process", time.Second)
		s.NoError(err)
		s.Equal("undefined", result)
	})

	s.Run("window is not defined", func() {
		_, err := s.Evaluate("window.location", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})
}

func (s *EvaluatorSecuritySuite) TestNoTimerAbuse() {
	s.Run("setTimeout is not defined", func() {
		_, err := s.Evaluate("setTimeout(() => {}, 1000)", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})

	s.Run("setInterval is not defined", func() {
		_, err := s.Evaluate("setInterval(() => {}, 1000)", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})

	s.Run("setImmediate is not defined", func() {
		_, err := s.Evaluate("setImmediate(() => {})", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "not defined")
	})
}

func (s *EvaluatorSecuritySuite) TestNoEvalAbuse() {
	s.Run("eval with process access fails", func() {
		_, err := s.Evaluate("eval('process.env')", time.Second)
		s.Error(err)
	})

	s.Run("Function constructor with process access fails", func() {
		_, err := s.Evaluate("new Function('return process.env')()", time.Second)
		s.Error(err)
	})
}

func (s *EvaluatorSecuritySuite) TestTimeoutEnforcement() {
	s.Run("infinite loop is interrupted", func() {
		_, err := s.Evaluate("while(true) {}", 100*time.Millisecond)
		s.Error(err)
		s.Contains(err.Error(), "timeout")
	})

	s.Run("long computation is interrupted", func() {
		script := `
			let x = 0;
			for (let i = 0; i < 1e12; i++) {
				x += i;
			}
			x;
		`
		_, err := s.Evaluate(script, 100*time.Millisecond)
		s.Error(err)
		s.Contains(err.Error(), "timeout")
	})

	s.Run("default timeout is applied for zero value", func() {
		// This test verifies the timeout normalization
		result, err := s.Evaluate("'quick'", 0)
		s.NoError(err)
		s.Equal("quick", result)
	})

	s.Run("max timeout is enforced", func() {
		// Verify that timeout > MaxTimeout gets capped
		timeout := normalizeTimeout(10 * time.Hour)
		s.Equal(MaxTimeout, timeout)
	})
}

func (s *EvaluatorSecuritySuite) TestStackOverflowProtection() {
	s.Run("infinite recursion is prevented", func() {
		script := `
			function recurse() {
				return recurse();
			}
			recurse();
		`
		_, err := s.Evaluate(script, time.Second)
		s.Require().Error(err, "infinite recursion should cause an error")
		// The error message contains the function name where overflow occurred
		s.Contains(err.Error(), "recurse")
	})

	s.Run("deep but finite recursion within limit works", func() {
		script := `
			function countdown(n) {
				if (n <= 0) return 0;
				return countdown(n - 1);
			}
			countdown(100);
		`
		result, err := s.Evaluate(script, time.Second)
		s.NoError(err)
		s.Equal("0", result)
	})
}

func (s *EvaluatorSecuritySuite) TestSafeBuiltinsAvailable() {
	s.Run("JSON is available", func() {
		result, err := s.Evaluate("JSON.stringify({a: 1})", time.Second)
		s.NoError(err)
		s.Equal(`{"a":1}`, result)
	})

	s.Run("Math is available", func() {
		result, err := s.Evaluate("Math.max(1, 2, 3)", time.Second)
		s.NoError(err)
		s.Equal("3", result)
	})

	s.Run("Array methods are available", func() {
		result, err := s.Evaluate("[1,2,3].map(x => x * 2).join(',')", time.Second)
		s.NoError(err)
		s.Equal("2,4,6", result)
	})

	s.Run("Object methods are available", func() {
		result, err := s.Evaluate("Object.keys({a:1, b:2}).join(',')", time.Second)
		s.NoError(err)
		s.Equal("a,b", result)
	})

	s.Run("String methods are available", func() {
		result, err := s.Evaluate("'hello'.toUpperCase()", time.Second)
		s.NoError(err)
		s.Equal("HELLO", result)
	})

	s.Run("Date is available", func() {
		result, err := s.Evaluate("typeof Date", time.Second)
		s.NoError(err)
		s.Equal("function", result)
	})

	s.Run("RegExp is available", func() {
		result, err := s.Evaluate("/test/.test('testing')", time.Second)
		s.NoError(err)
		s.Equal("true", result)
	})
}

func (s *EvaluatorSecuritySuite) TestErrorMessages() {
	s.Run("syntax errors are descriptive", func() {
		_, err := s.Evaluate("function(", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "Unexpected token")
	})

	s.Run("runtime errors are descriptive", func() {
		_, err := s.Evaluate("throw new Error('custom error')", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "custom error")
	})

	s.Run("reference errors are descriptive", func() {
		_, err := s.Evaluate("undefinedVariable", time.Second)
		s.Error(err)
		s.Contains(err.Error(), "undefinedVariable is not defined")
	})
}

func TestEvaluatorSecurity(t *testing.T) {
	suite.Run(t, new(EvaluatorSecuritySuite))
}
