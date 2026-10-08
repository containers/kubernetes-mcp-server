package mcp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type InFlightMiddlewareSuite struct {
	suite.Suite
}

func (s *InFlightMiddlewareSuite) TestGlobalSlotExhaustion() {
	s.Run("one pool rejects the next limited call and ignores other methods", func() {
		var limit atomic.Int32
		limit.Store(1)
		block := make(chan struct{})
		entered := make(chan struct{}, 1)
		handler := inFlightMiddleware(func() int { return int(limit.Load()) })(func(_ context.Context, method string, _ mcp.Request) (mcp.Result, error) {
			if method == "tools/call" {
				entered <- struct{}{}
				<-block
			}
			return nil, nil
		})
		req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{}}

		errCh := make(chan error, 1)
		go func() {
			_, err := handler(context.Background(), "tools/call", req)
			errCh <- err
		}()
		waitEntered(s.T(), entered)
		defer func() {
			close(block)
			s.NoError(<-errCh)
		}()

		_, err := handler(context.Background(), "prompts/get", req)
		s.requireInFlightError(err)

		_, err = handler(context.Background(), "resources/read", req)
		s.requireInFlightError(err)

		_, err = handler(context.Background(), "tools/list", req)
		s.NoError(err)

		limit.Store(2)
		_, err = handler(context.Background(), "prompts/get", req)
		s.NoError(err, "a raised limit is read on the next acquire")

		limit.Store(1)
		_, err = handler(context.Background(), "resources/read", req)
		s.requireInFlightError(err)
	})

	s.Run("releases the slot when the handler returns an error", func() {
		handler := inFlightMiddleware(func() int { return 1 })(func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, errors.New("tool failed")
		})
		req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{}}
		for range 2 {
			_, err := handler(context.Background(), "tools/call", req)
			s.Require().Error(err)
			s.Contains(err.Error(), "tool failed")
			s.NotContains(err.Error(), "max_in_flight")
		}
	})

	s.Run("releases the slot when the handler panics", func() {
		panicNext := true
		handler := inFlightMiddleware(func() int { return 1 })(func(context.Context, string, mcp.Request) (mcp.Result, error) {
			if panicNext {
				panicNext = false
				panic("boom")
			}
			return nil, nil
		})
		req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{}}
		s.Panics(func() {
			_, _ = handler(context.Background(), "tools/call", req)
		})
		_, err := handler(context.Background(), "tools/call", req)
		s.NoError(err)
	})

	s.Run("zero disables the limit", func() {
		var running atomic.Int32
		started := make(chan struct{}, 8)
		release := make(chan struct{})
		handler := inFlightMiddleware(func() int { return 0 })(func(context.Context, string, mcp.Request) (mcp.Result, error) {
			running.Add(1)
			started <- struct{}{}
			<-release
			running.Add(-1)
			return nil, nil
		})
		req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{}}
		errCh := make(chan error, 8)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				_, err := handler(context.Background(), "tools/call", req)
				errCh <- err
			})
		}
		for range 8 {
			waitEntered(s.T(), started)
		}
		s.Equal(int32(8), running.Load())
		close(release)
		wg.Wait()
		close(errCh)
		for err := range errCh {
			s.NoError(err)
		}
	})
}

func (s *InFlightMiddlewareSuite) requireInFlightError(err error) {
	s.T().Helper()
	s.Require().Error(err)
	var rpcErr *jsonrpc.Error
	s.Require().ErrorAs(err, &rpcErr)
	s.Equal(int64(CodeInFlightLimitExceeded), rpcErr.Code)
	s.Contains(err.Error(), "max_in_flight limit exceeded")
}

func TestInFlightMiddleware(t *testing.T) {
	suite.Run(t, new(InFlightMiddlewareSuite))
}

type InFlightServerSuite struct {
	BaseMcpSuite
}

func (s *InFlightServerSuite) TestServerRejectsWhenSlotsAreExhausted() {
	s.Run("a second session is rejected until the first call returns", func() {
		configtest.OverlayTOML(s.T(), &s.Cfg, `
max_in_flight = 1

[[confirmation_rules]]
tool = "pods_list"
message = "List pods?"
`)
		release := make(chan struct{})
		var releaseOnce sync.Once
		stop := func() { releaseOnce.Do(func() { close(release) }) }
		var done chan struct{}
		defer func() {
			stop()
			if done == nil {
				return
			}
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				s.Fail("first call did not finish")
			}
		}()

		held := make(chan struct{})
		var heldOnce sync.Once
		s.InitMcpClient(test.WithElicitationHandler(
			func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				heldOnce.Do(func() { close(held) })
				<-release
				return &mcp.ElicitResult{Action: "accept"}, nil
			},
		))

		done = make(chan struct{})
		var firstResult *mcp.CallToolResult
		var firstErr error
		go func() {
			defer close(done)
			firstResult, firstErr = s.CallTool("pods_list", map[string]any{})
		}()
		waitEntered(s.T(), held)

		client2 := test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP())
		defer client2.Close()

		tools, err := client2.ListTools()
		s.Require().NoError(err)
		s.NotEmpty(tools.Tools)

		_, err = client2.CallTool("pods_list", map[string]any{})
		s.requireInFlightError(err)

		_, err = client2.GetPrompt("missing", nil)
		s.requireInFlightError(err)

		_, err = client2.ReadResource("missing://resource")
		s.requireInFlightError(err)

		s.Cfg.MaxInFlight.SetForTest(2)
		raised, err := client2.CallTool("pods_list", map[string]any{})
		s.Require().NoError(err, "a raised limit is read on the next acquire")
		s.Require().NotNil(raised)
		s.False(raised.IsError)

		stop()
		waitEntered(s.T(), done)
		s.Require().NoError(firstErr)
		s.Require().NotNil(firstResult)
		s.False(firstResult.IsError)
	})
}

func (s *InFlightServerSuite) requireInFlightError(err error) {
	s.T().Helper()
	s.Require().Error(err)
	var rpcErr *jsonrpc.Error
	s.Require().ErrorAs(err, &rpcErr)
	s.Equal(int64(CodeInFlightLimitExceeded), rpcErr.Code)
	s.Contains(err.Error(), "max_in_flight limit exceeded")
}

func TestInFlightServer(t *testing.T) {
	suite.Run(t, new(InFlightServerSuite))
}

func waitEntered(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the in-flight call to start")
	}
}
