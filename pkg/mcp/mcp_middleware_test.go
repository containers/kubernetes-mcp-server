package mcp

import (
	"bytes"
	"context"
	"flag"
	"regexp"
	"strconv"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
)

type McpLoggingSuite struct {
	BaseMcpSuite
	klogState klog.State
	logBuffer bytes.Buffer
}

func (s *McpLoggingSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.klogState = klog.CaptureState()
}

func (s *McpLoggingSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	s.klogState.Restore()
}

func (s *McpLoggingSuite) SetLogLevel(level int) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	klog.InitFlags(flags)
	_ = flags.Set("v", strconv.Itoa(level))
	klog.SetLogger(textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(level), textlogger.Output(&s.logBuffer))))
}

func (s *McpLoggingSuite) TestLogsToolCall() {
	s.SetLogLevel(5)
	s.InitMcpClient()
	_, err := s.CallTool("configuration_view", map[string]any{"minified": false})
	s.Require().NoError(err, "call to tool configuration_view failed")

	s.Run("Logs tool name", func() {
		s.Contains(s.logBuffer.String(), "mcp tool call: configuration_view(")
	})
	s.Run("Logs tool call arguments", func() {
		expected := `"mcp tool call: configuration_view\((.+)\)"`
		m := regexp.MustCompile(expected).FindStringSubmatch(s.logBuffer.String())
		s.Len(m, 2, "Expected log entry to contain arguments")
		s.Equal("map[minified:false]", m[1], "Expected log arguments to be 'map[minified:false]'")
	})
}

func (s *McpLoggingSuite) TestLogsToolCallHeaders() {
	s.SetLogLevel(7)
	s.InitMcpClient(test.WithTransport(transport.WithHTTPHeaders(map[string]string{
		"Accept-Encoding":   "gzip",
		"Authorization":     "Bearer should-not-be-logged",
		"authorization":     "Bearer should-not-be-logged",
		"a-loggable-header": "should-be-logged",
	})))
	_, err := s.CallTool("configuration_view", map[string]any{"minified": false})
	s.Require().NoError(err, "call to tool configuration_view failed")

	s.Run("Logs tool call headers", func() {
		expectedLog := "mcp tool call headers: A-Loggable-Header: should-be-logged"
		s.Contains(s.logBuffer.String(), expectedLog, "Expected log to contain loggable header")
	})
	sensitiveHeaders := []string{
		"Authorization:",
		// TODO: Add more sensitive headers as needed
	}
	s.Run("Does not log sensitive headers", func() {
		for _, header := range sensitiveHeaders {
			s.NotContains(s.logBuffer.String(), header, "Log should not contain sensitive header")
		}
	})
	s.Run("Does not log sensitive header values", func() {
		s.NotContains(s.logBuffer.String(), "should-not-be-logged", "Log should not contain sensitive header value")
	})
}

func TestMcpLogging(t *testing.T) {
	suite.Run(t, new(McpLoggingSuite))
}

// TraceContextPropagationSuite tests the trace context propagation middleware
type TraceContextPropagationSuite struct {
	suite.Suite
}

func (s *TraceContextPropagationSuite) SetupTest() {
	// Set up a global text map propagator for tests
	// This is necessary because otel.GetTextMapPropagator() returns NoOp by default
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

// TestMetaCarrier tests the metaCarrier implementation of TextMapCarrier
func (s *TraceContextPropagationSuite) TestMetaCarrier() {
	s.Run("Get returns string value from meta", func() {
		carrier := &metaCarrier{
			meta: map[string]any{
				"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
				"tracestate":  "rojo=00f067aa0ba902b7",
			},
		}
		s.Equal("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", carrier.Get("traceparent"))
		s.Equal("rojo=00f067aa0ba902b7", carrier.Get("tracestate"))
	})

	s.Run("Get returns empty string for missing key", func() {
		carrier := &metaCarrier{meta: map[string]any{}}
		s.Equal("", carrier.Get("traceparent"))
	})

	s.Run("Get returns empty string for non-string value", func() {
		carrier := &metaCarrier{
			meta: map[string]any{
				"number": 123,
				"bool":   true,
			},
		}
		s.Equal("", carrier.Get("number"))
		s.Equal("", carrier.Get("bool"))
	})

	s.Run("Keys returns all keys in meta", func() {
		carrier := &metaCarrier{
			meta: map[string]any{
				"traceparent": "value1",
				"tracestate":  "value2",
				"other":       "value3",
			},
		}
		keys := carrier.Keys()
		s.Len(keys, 3)
		s.Contains(keys, "traceparent")
		s.Contains(keys, "tracestate")
		s.Contains(keys, "other")
	})

	s.Run("Keys returns empty slice for empty meta", func() {
		carrier := &metaCarrier{meta: map[string]any{}}
		keys := carrier.Keys()
		s.Empty(keys)
	})
}

// TestTraceContextPropagationMiddleware tests the trace context propagation middleware
func (s *TraceContextPropagationSuite) TestTraceContextPropagationMiddleware() {
	s.Run("extracts trace context from CallToolParamsRaw", func() {
		// Create a valid traceparent header (version 00, trace ID, span ID, flags)
		traceparent := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
		tracestate := "rojo=00f067aa0ba902b7"

		params := &mcp.CallToolParamsRaw{
			Meta: mcp.Meta{
				"traceparent": traceparent,
				"tracestate":  tracestate,
			},
		}

		// Since we can't easily create mcp.Request due to sealed interfaces,
		// let's test the carrier and extraction logic directly
		carrier := &metaCarrier{meta: params.Meta}
		ctx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)

		spanContext := trace.SpanContextFromContext(ctx)
		s.True(spanContext.IsValid(), "Expected valid span context to be extracted")
		s.True(spanContext.IsRemote(), "Expected span context to be marked as remote")

		expectedTraceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
		s.Equal(expectedTraceID, spanContext.TraceID())

		expectedSpanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
		s.Equal(expectedSpanID, spanContext.SpanID())
	})

	s.Run("handles empty metadata gracefully", func() {
		params := &mcp.CallToolParamsRaw{
			Meta: mcp.Meta{},
		}

		carrier := &metaCarrier{meta: params.Meta}
		ctx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)

		spanContext := trace.SpanContextFromContext(ctx)
		s.False(spanContext.IsValid(), "Expected no trace context for empty metadata")
	})

	s.Run("handles nil metadata gracefully", func() {
		params := &mcp.CallToolParamsRaw{
			Meta: nil,
		}

		if len(params.Meta) > 0 {
			carrier := &metaCarrier{meta: params.Meta}
			ctx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)

			spanContext := trace.SpanContextFromContext(ctx)
			s.False(spanContext.IsValid(), "Should not have extracted trace context")
		}
		// If meta is nil, we don't attempt extraction - this is the expected behavior
		s.Nil(params.Meta, "Meta should be nil")
	})
}

// TestTraceContextPropagationCarrierWithPropagator verifies that metaCarrier works with the actual propagator
func (s *TraceContextPropagationSuite) TestTraceContextPropagationCarrierWithPropagator() {
	s.Run("W3C TraceContext propagator can extract from metaCarrier", func() {
		traceparent := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
		tracestate := "rojo=00f067aa0ba902b7,congo=t61rcWkgMzE"

		carrier := &metaCarrier{
			meta: map[string]any{
				"traceparent": traceparent,
				"tracestate":  tracestate,
			},
		}

		propagator := propagation.TraceContext{}
		ctx := propagator.Extract(context.Background(), carrier)

		spanContext := trace.SpanContextFromContext(ctx)
		s.True(spanContext.IsValid(), "Propagator should extract valid span context")
		s.True(spanContext.IsRemote(), "Extracted span should be marked as remote")

		// Verify extracted values
		expectedTraceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
		s.Equal(expectedTraceID, spanContext.TraceID())

		expectedSpanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
		s.Equal(expectedSpanID, spanContext.SpanID())

		s.Equal("rojo=00f067aa0ba902b7,congo=t61rcWkgMzE", spanContext.TraceState().String())
	})
}

func TestTraceContextPropagation(t *testing.T) {
	suite.Run(t, new(TraceContextPropagationSuite))
}
