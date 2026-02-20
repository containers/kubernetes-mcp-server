package code

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// EvaluatorContextSuite tests security aspects of context handling in the JavaScript evaluator.
type EvaluatorContextSuite struct {
	BaseEvaluatorSuite
}

func (s *EvaluatorContextSuite) TestContextValuesNotAccessibleFromJavaScript() {
	type contextKey string
	const (
		authTokenKey contextKey = "auth_token"
		userInfoKey  contextKey = "user_info"
	)

	ctx := context.WithValue(context.Background(), authTokenKey, "super-secret-token")
	ctx = context.WithValue(ctx, userInfoKey, map[string]string{
		"username": "admin",
		"role":     "cluster-admin",
	})

	evaluator, err := NewEvaluator(ctx, s.kubernetes)
	s.Require().NoError(err)

	result, err := evaluator.Evaluate(`
		const results = {
			valueResult: ctx.value("auth_token"),
			valueResult2: ctx.value("user_info"),
			ctxKeys: Object.keys(ctx || {}),
			ctxType: typeof ctx
		};
		JSON.stringify(results);
	`, time.Second)
	s.NoError(err)
	s.T().Logf("Context access result: %s", result)

	s.NotContains(result, "super-secret-token")
	s.NotContains(result, "admin")
	s.NotContains(result, "cluster-admin")
	s.Contains(result, `"valueResult":null`)
	s.Contains(result, `"valueResult2":null`)
}

func (s *EvaluatorContextSuite) TestContextCancellationMethodsAreExposed() {
	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evaluator, err := NewEvaluator(cancelCtx, s.kubernetes)
	s.Require().NoError(err)

	result, err := evaluator.Evaluate(`
		const done = ctx.done();
		done !== null && done !== undefined ? "has done" : "no done";
	`, time.Second)
	s.NoError(err)
	s.Equal("has done", result)

	result, err = evaluator.Evaluate(`
		typeof ctx.err === 'function' ? "has err method" : "no err method";
	`, time.Second)
	s.NoError(err)
	s.Equal("has err method", result)

	result, err = evaluator.Evaluate(`
		typeof ctx.deadline === 'function' ? "has deadline method" : "no deadline method";
	`, time.Second)
	s.NoError(err)
	s.Equal("has deadline method", result)
}

func (s *EvaluatorContextSuite) TestContextDeadlineIsAccessible() {
	deadline := time.Now().Add(5 * time.Minute)
	deadlineCtx, cancelFunc := context.WithDeadline(context.Background(), deadline)
	defer cancelFunc()

	evaluator, err := NewEvaluator(deadlineCtx, s.kubernetes)
	s.Require().NoError(err)

	result, err := evaluator.Evaluate(`
		const [dl, ok] = ctx.deadline();
		ok ? "has deadline" : "no deadline";
	`, time.Second)
	s.NoError(err)
	s.Equal("has deadline", result)
}

func (s *EvaluatorContextSuite) TestSafeContextValueAlwaysReturnsNil() {
	type contextKey string
	const secretKey contextKey = "secret"

	originalCtx := context.WithValue(context.Background(), secretKey, "sensitive-data")
	safeCtx := newSafeContext(originalCtx)

	s.Equal("sensitive-data", originalCtx.Value(secretKey))
	s.Nil(safeCtx.Value(secretKey))
	s.Nil(safeCtx.Value("any-key"))
	s.Nil(safeCtx.Value(123))
}

func (s *EvaluatorContextSuite) TestSafeContextDoneChannelIsPreserved() {
	cancelCtx, cancel := context.WithCancel(context.Background())
	safeCtx := newSafeContext(cancelCtx)

	select {
	case <-safeCtx.Done():
		s.Fail("Done should not be closed yet")
	default:
	}

	cancel()

	select {
	case <-safeCtx.Done():
	case <-time.After(100 * time.Millisecond):
		s.Fail("Done should be closed after cancel")
	}
}

func (s *EvaluatorContextSuite) TestSafeContextErrIsPreserved() {
	cancelCtx, cancel := context.WithCancel(context.Background())
	safeCtx := newSafeContext(cancelCtx)

	s.Nil(safeCtx.Err())
	cancel()
	s.Equal(context.Canceled, safeCtx.Err())
}

func (s *EvaluatorContextSuite) TestSafeContextDeadlineIsPreserved() {
	deadline := time.Now().Add(time.Hour)
	deadlineCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	safeCtx := newSafeContext(deadlineCtx)

	gotDeadline, ok := safeCtx.Deadline()
	s.True(ok)
	s.Equal(deadline, gotDeadline)
}

func TestEvaluatorContext(t *testing.T) {
	suite.Run(t, new(EvaluatorContextSuite))
}
