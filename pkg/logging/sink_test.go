package logging_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/logtest"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/logging"
	"github.com/containers/kubernetes-mcp-server/pkg/telemetry"
)

// SinkSuite tests do not call t.Parallel(): every test mutates klog's global
// logger via logging.New, and CaptureState/Restore is a process-global
// snapshot. Cross-package parallelism is fine (Go runs each package in its
// own process) — within-package parallelism would corrupt klog state.

type SinkSuite struct {
	suite.Suite
	tempDir   string
	httpOut   *test.SyncBuffer
	errOut    *test.SyncBuffer
	klogState klog.State
}

func (s *SinkSuite) SetupTest() {
	s.tempDir = s.T().TempDir()
	s.httpOut = &test.SyncBuffer{}
	s.errOut = &test.SyncBuffer{}
	s.klogState = klog.CaptureState()
}

func (s *SinkSuite) TearDownTest() {
	s.klogState.Restore()
}

// newSink wraps logging.New with the suite's IO buffers and a require-no-error.
func (s *SinkSuite) newSink(cfg *config.StaticConfig) *logging.Sink {
	sink, err := logging.New(cfg, s.httpOut, s.errOut)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sink.Close() })
	return sink
}

func (s *SinkSuite) TestNewRoutesToConfiguredDestination() {
	s.Run("stdio mode without log_file discards klog output", func() {
		sink := s.newSink(&config.StaticConfig{LogLevel: 1})
		klog.V(1).Info("should be discarded")
		klog.Flush()
		_, err := sink.Write([]byte("direct write"))
		s.Require().NoError(err)
		s.Empty(s.httpOut.String(), "stdio mode must not write to httpOut (it would corrupt the protocol channel)")
		s.Empty(s.errOut.String(), "stdio mode must not write to errOut without explicit opt-in")
	})

	s.Run("HTTP mode without log_file routes to httpOut", func() {
		s.newSink(&config.StaticConfig{LogLevel: 1, Port: "8080"})
		klog.V(1).Info("hello-http")
		klog.Flush()
		s.Contains(s.httpOut.String(), "hello-http")
	})

	s.Run("stderr sentinel routes to errOut without opening a file", func() {
		s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: logging.StderrSentinel})
		klog.V(1).Info("hello-stderr")
		klog.Flush()
		s.Contains(s.errOut.String(), "hello-stderr")
	})

	s.Run("log_file path opens a file and routes to it", func() {
		path := filepath.Join(s.tempDir, "server.log")
		s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: path})
		klog.V(1).Info("hello-file")
		klog.Flush()
		content, err := os.ReadFile(path)
		s.Require().NoError(err)
		s.Contains(string(content), "hello-file")
	})

	s.Run("zero-value config is the same as stdio with no log_file", func() {
		// Earlier subtests in this group share s.httpOut/s.errOut — reset
		// before asserting "no writes happened".
		s.httpOut.Reset()
		s.errOut.Reset()
		s.newSink(&config.StaticConfig{})
		klog.V(1).Info("zero-config")
		klog.Flush()
		s.Empty(s.httpOut.String())
		s.Empty(s.errOut.String())
	})
}

func (s *SinkSuite) TestNewWithBadLogFilePathFails() {
	s.Run("returns wrapped error and does not panic", func() {
		_, err := logging.New(
			&config.StaticConfig{LogFile: filepath.Join(s.tempDir, "missing", "server.log")},
			s.httpOut, s.errOut,
		)
		s.Require().Error(err)
		s.Contains(err.Error(), "failed to open log file")
	})
}

func (s *SinkSuite) TestReloadSwitchesLogFile() {
	pathA := filepath.Join(s.tempDir, "a.log")
	pathB := filepath.Join(s.tempDir, "b.log")
	sink := s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: pathA})

	klog.V(1).Info("first")
	klog.Flush()

	s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 1, LogFile: pathB}))

	klog.V(1).Info("second")
	klog.Flush()

	s.Run("first message landed in file A", func() {
		content, err := os.ReadFile(pathA)
		s.Require().NoError(err)
		s.Contains(string(content), "first")
		s.NotContains(string(content), "second")
	})

	s.Run("second message landed in file B", func() {
		content, err := os.ReadFile(pathB)
		s.Require().NoError(err)
		s.Contains(string(content), "second")
		s.NotContains(string(content), "first")
	})
}

func (s *SinkSuite) TestReloadAfterRotationCreatesNewInode() {
	// Simulates the logrotate flow:
	//   1. server writes to server.log
	//   2. logrotate renames server.log -> server.log.1
	//   3. SIGHUP -> Reload reopens server.log (a fresh inode)
	//   4. subsequent writes land in the new inode, not the rotated one
	//
	// Windows refuses to rename a file that is currently open for writing,
	// so the rename-while-open step is Unix-only. logrotate is itself a
	// Unix tool, so the behavior under test does not apply on Windows.
	if runtime.GOOS == "windows" {
		s.T().Skip("rename-while-open is not supported on Windows; logrotate flow is Unix-only")
	}
	logFile := filepath.Join(s.tempDir, "server.log")
	rotated := logFile + ".1"
	sink := s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: logFile})

	klog.V(1).Info("before-rotate")
	klog.Flush()

	s.Require().NoError(os.Rename(logFile, rotated))
	s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 1, LogFile: logFile}))

	klog.V(1).Info("after-rotate")
	klog.Flush()

	s.Run("rotated file holds the pre-rotation entry", func() {
		content, err := os.ReadFile(rotated)
		s.Require().NoError(err)
		s.Contains(string(content), "before-rotate")
	})

	s.Run("new file at original path holds the post-rotation entry", func() {
		content, err := os.ReadFile(logFile)
		s.Require().NoError(err)
		s.Contains(string(content), "after-rotate")
		s.NotContains(string(content), "before-rotate")
	})

	s.Run("new file is a different inode than the rotated one", func() {
		newInfo, err := os.Stat(logFile)
		s.Require().NoError(err)
		rotatedInfo, err := os.Stat(rotated)
		s.Require().NoError(err)
		s.False(os.SameFile(newInfo, rotatedInfo))
	})
}

func (s *SinkSuite) TestReloadKeepsOldDestinationOnError() {
	pathA := filepath.Join(s.tempDir, "a.log")
	sink := s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: pathA})

	bad := &config.StaticConfig{LogLevel: 1, LogFile: filepath.Join(s.tempDir, "missing", "server.log")}
	err := sink.Reload(bad)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to open log file")

	klog.V(1).Info("after-failed-reload")
	klog.Flush()

	s.Run("logs continue to land in the original file", func() {
		content, err := os.ReadFile(pathA)
		s.Require().NoError(err)
		s.Contains(string(content), "after-failed-reload")
	})
}

func (s *SinkSuite) TestReloadSwitchesToStderr() {
	pathA := filepath.Join(s.tempDir, "a.log")
	sink := s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: pathA})

	s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 1, LogFile: logging.StderrSentinel}))

	klog.V(1).Info("on-stderr")
	klog.Flush()

	s.Run("subsequent logs go to errOut, not the previous file", func() {
		s.Contains(s.errOut.String(), "on-stderr")
		content, _ := os.ReadFile(pathA)
		s.NotContains(string(content), "on-stderr")
	})
}

func (s *SinkSuite) TestReloadUpdatesVerbosity() {
	pathA := filepath.Join(s.tempDir, "a.log")
	sink := s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: pathA})

	klog.V(3).Info("level3-before")
	klog.Flush()

	s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 4, LogFile: pathA}))

	klog.V(3).Info("level3-after")
	klog.Flush()

	content, err := os.ReadFile(pathA)
	s.Require().NoError(err)

	s.Run("V(3) suppressed at log_level=1", func() {
		s.NotContains(string(content), "level3-before")
	})
	s.Run("V(3) emitted after raising log_level to 4", func() {
		s.Contains(string(content), "level3-after")
	})
}

func (s *SinkSuite) TestSDKLoggerFollowsReload() {
	pathA := filepath.Join(s.tempDir, "a.log")
	pathB := filepath.Join(s.tempDir, "b.log")
	sink := s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: pathA})

	sink.SDKLogger().Info("sdk-on-a")
	klog.Flush()
	s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 1, LogFile: pathB}))
	sink.SDKLogger().Info("sdk-on-b")
	klog.Flush()

	s.Run("first SDK message landed in file A", func() {
		content, err := os.ReadFile(pathA)
		s.Require().NoError(err)
		s.Contains(string(content), "sdk-on-a")
	})
	s.Run("second SDK message landed in file B", func() {
		content, err := os.ReadFile(pathB)
		s.Require().NoError(err)
		s.Contains(string(content), "sdk-on-b")
	})
}

func (s *SinkSuite) TestConcurrentWriteAndReloadIsRaceFree() {
	// This test has no Equal/True assertions on purpose: it is a probe for
	// the race detector. The package's whole reason to exist is that
	// concurrent klog.V(...) calls and sink.Reload(...) must not race
	// against klog or the writer pointer. A regression that re-introduced
	// klog.SetLoggerWithOptions on the reload path would surface here under
	// `go test -race`. Run the loops long enough that the detector has real
	// surface area to inspect.
	pathA := filepath.Join(s.tempDir, "a.log")
	pathB := filepath.Join(s.tempDir, "b.log")
	sink := s.newSink(&config.StaticConfig{LogLevel: 2, LogFile: pathA})

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					klog.V(1).Info("hot-loop")
					runtime.Gosched()
				}
			}
		}()
	}

	for i := 0; i < 1000; i++ {
		path := pathA
		if i%2 == 1 {
			path = pathB
		}
		level := 1 + i%4
		s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: level, LogFile: path}))
	}
	close(stop)
	wg.Wait()
}

func (s *SinkSuite) TestConcurrentWriteAndReloadNeverWritesToClosedFile() {
	// Reload closes the previously-open file inline (sink.go applyDestination).
	// Without mutual exclusion between Write and that close, a Write that has
	// already loaded the old writer can land on the just-closed descriptor: on
	// POSIX that surfaces as an os.ErrClosed write error, and on Windows it can
	// wedge the runtime poller and deadlock the whole package (issue #1199).
	// Drive direct Sink.Write (bypassing klog's global serialization, so the
	// writers are genuinely concurrent) against a tight Reload loop and assert
	// that no write ever observes a closed descriptor.
	pathA := filepath.Join(s.tempDir, "a.log")
	pathB := filepath.Join(s.tempDir, "b.log")
	sink := s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: pathA})

	stop := make(chan struct{})
	var writeErr atomic.Value // first non-nil error observed by any writer
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if _, err := sink.Write([]byte("hot-loop\n")); err != nil {
						writeErr.CompareAndSwap(nil, err)
					}
					runtime.Gosched()
				}
			}
		}()
	}

	for i := 0; i < 2000; i++ {
		path := pathA
		if i%2 == 1 {
			path = pathB
		}
		s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 1, LogFile: path}))
	}
	close(stop)
	wg.Wait()

	err, _ := writeErr.Load().(error)
	s.NoError(err, "a concurrent Write landed on a descriptor that Reload had already closed")
}

func (s *SinkSuite) TestReloadIgnoresPortChangeForServeMode() {
	// The serve mode (HTTP vs stdio) is decided once in cmd.Run and locked
	// in for the process lifetime. A SIGHUP-reloaded config that flipped
	// Port must NOT flip the log destination — otherwise a process running
	// in stdio mode whose config grows a Port would start writing klog to
	// stdout, corrupting the MCP protocol channel.
	sink := s.newSink(&config.StaticConfig{LogLevel: 1}) // stdio mode (no Port)

	s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 1, Port: "8080"}))

	klog.V(1).Info("after-reload-with-port")
	klog.Flush()

	s.Run("logs are still discarded, not routed to httpOut", func() {
		s.Empty(s.httpOut.String(), "stdio-mode sink must keep discarding even after a reload that adds Port")
	})
}

func (s *SinkSuite) TestReloadDropsLogFileBackToDefault() {
	// Operators sometimes set log_file via TOML and later remove it on
	// SIGHUP. The sink should revert to the default destination (httpOut in
	// HTTP mode, discard in stdio).
	pathA := filepath.Join(s.tempDir, "a.log")
	sink := s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: pathA, Port: "8080"})

	klog.V(1).Info("on-file")
	klog.Flush()
	s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 1, Port: "8080"}))
	klog.V(1).Info("on-default")
	klog.Flush()

	s.Run("first message landed in the file", func() {
		content, err := os.ReadFile(pathA)
		s.Require().NoError(err)
		s.Contains(string(content), "on-file")
	})
	s.Run("second message landed on httpOut after dropping log_file", func() {
		s.Contains(s.httpOut.String(), "on-default")
	})
}

func (s *SinkSuite) TestReloadAfterCloseReopensQuietly() {
	// Close-then-Reload is not a documented pattern, but it's reachable if
	// shutdown overlaps a SIGHUP. The Sink should not panic; behavior is to
	// quietly install a new destination.
	pathA := filepath.Join(s.tempDir, "a.log")
	pathB := filepath.Join(s.tempDir, "b.log")
	sink, err := logging.New(&config.StaticConfig{LogFile: pathA}, s.httpOut, s.errOut)
	s.Require().NoError(err)
	s.Require().NoError(sink.Close())
	s.Require().NoError(sink.Reload(&config.StaticConfig{LogFile: pathB}))
	s.Require().NoError(sink.Close())
}

func (s *SinkSuite) TestDoubleReloadToSamePath() {
	pathA := filepath.Join(s.tempDir, "a.log")
	sink := s.newSink(&config.StaticConfig{LogLevel: 1, LogFile: pathA})

	s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 1, LogFile: pathA}))
	s.Require().NoError(sink.Reload(&config.StaticConfig{LogLevel: 1, LogFile: pathA}))

	klog.V(1).Info("after-double-reload")
	klog.Flush()
	content, err := os.ReadFile(pathA)
	s.Require().NoError(err)
	s.Contains(string(content), "after-double-reload")
}

func (s *SinkSuite) TestCloseRoutesPostCloseLogsToErrOut() {
	// After Close, the file fd is released — without rerouting the writer,
	// any subsequent klog call (notably the deferred error log in cmd's
	// RunE when Close itself errors) would hit a closed fd and be silently
	// swallowed. Pin the contract: post-Close logs land on errOut.
	pathA := filepath.Join(s.tempDir, "a.log")
	sink, err := logging.New(&config.StaticConfig{LogLevel: 1, LogFile: pathA}, s.httpOut, s.errOut)
	s.Require().NoError(err)
	s.Require().NoError(sink.Close())

	klog.V(1).Info("after-close")
	klog.Flush()

	s.Contains(s.errOut.String(), "after-close")
}

func (s *SinkSuite) TestCloseIsIdempotent() {
	pathA := filepath.Join(s.tempDir, "a.log")
	sink, err := logging.New(&config.StaticConfig{LogFile: pathA}, s.httpOut, s.errOut)
	s.Require().NoError(err)
	s.Require().NoError(sink.Close())
	s.Require().NoError(sink.Close(), "second Close must be a no-op, not panic")
}

func (s *SinkSuite) TestSinkImplementsIoWriter() {
	// Compile-time assertion plus a smoke test that direct writes route to
	// the configured destination without going through klog.
	var _ io.Writer = (*logging.Sink)(nil)
	pathA := filepath.Join(s.tempDir, "a.log")
	sink := s.newSink(&config.StaticConfig{LogFile: pathA})
	_, err := sink.Write([]byte("direct"))
	s.Require().NoError(err)
	content, err := os.ReadFile(pathA)
	s.Require().NoError(err)
	s.Contains(string(content), "direct")
}

func (s *SinkSuite) TestWithOtelLogSinkLogsToTextAndOtel() {
	// Wire the real NewLogSink through the tee and verify logs land in both
	// the text sink (file) and the OTel recorder.
	recorder := logtest.NewRecorder()
	otelSink := telemetry.NewLogSink("test-svc", "1.0.0", recorder)

	pathA := filepath.Join(s.tempDir, "a.log")
	sink, err := logging.New(
		&config.StaticConfig{LogLevel: 1, LogFile: pathA},
		s.httpOut, s.errOut,
		logging.WithOtelLogSink(otelSink, nil),
	)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sink.Close() })

	klog.FromContext(s.T().Context()).V(1).Info("hello-both", "k", "v")
	klog.Flush()

	s.Run("text sink receives the log", func() {
		content, err := os.ReadFile(pathA)
		s.Require().NoError(err)
		s.Contains(string(content), "hello-both")
	})

	s.Run("OTel recorder receives the log with correct body and attributes", func() {
		records := allRecords(recorder.Result())
		s.Require().NotEmpty(records, "expected at least one OTel record")
		s.Equal("hello-both", records[0].Body.AsString())
		s.True(hasAttr(records[0], "k", "v"))
	})
}

func (s *SinkSuite) TestWithOtelLogSinkErrorLandsInBoth() {
	recorder := logtest.NewRecorder()
	otelSink := telemetry.NewLogSink("test-svc", "1.0.0", recorder)

	pathA := filepath.Join(s.tempDir, "a.log")
	sink, err := logging.New(
		&config.StaticConfig{LogLevel: 1, LogFile: pathA},
		s.httpOut, s.errOut,
		logging.WithOtelLogSink(otelSink, nil),
	)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sink.Close() })

	klog.FromContext(s.T().Context()).Error(errSentinel, "something failed", "code", 42)
	klog.Flush()

	s.Run("text sink receives the error log", func() {
		content, err := os.ReadFile(pathA)
		s.Require().NoError(err)
		s.Contains(string(content), "something failed")
	})

	s.Run("OTel recorder receives the error log", func() {
		records := allRecords(recorder.Result())
		s.Require().NotEmpty(records)
		s.Equal("something failed", records[0].Body.AsString())
	})
}

func (s *SinkSuite) TestWithOtelLogSinkBelowVerbosityReachesNeitherSink() {
	// klog's own verbosity gate runs before the teeSink, so V(5) at
	// LogLevel=1 never reaches either the text sink or the OTel recorder.
	recorder := logtest.NewRecorder()
	otelSink := telemetry.NewLogSink("test-svc", "1.0.0", recorder)

	pathA := filepath.Join(s.tempDir, "a.log")
	sink, err := logging.New(
		&config.StaticConfig{LogLevel: 1, LogFile: pathA},
		s.httpOut, s.errOut,
		logging.WithOtelLogSink(otelSink, nil),
	)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sink.Close() })

	klog.FromContext(s.T().Context()).V(5).Info("too-verbose")
	klog.Flush()

	s.Run("text sink does not contain the suppressed log", func() {
		content, _ := os.ReadFile(pathA)
		s.NotContains(string(content), "too-verbose")
	})

	s.Run("OTel recorder does not contain the suppressed log", func() {
		records := allRecords(recorder.Result())
		s.Empty(records, "klog gates V(5) at LogLevel=1 before either sink")
	})
}

func (s *SinkSuite) TestOtelLogRecordCarriesTraceContext() {
	recorder := logtest.NewRecorder()
	otelSink := telemetry.NewLogSink("test-svc", "1.0.0", recorder)

	_, err := logging.New(
		&config.StaticConfig{LogLevel: 1, LogFile: logging.StderrSentinel},
		s.httpOut, s.errOut,
		logging.WithOtelLogSink(otelSink, nil),
	)
	s.Require().NoError(err)

	// Create a real TracerProvider and start a span so the context
	// looks exactly like production: a recording span with valid IDs.
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	s.T().Cleanup(func() { _ = tp.Shutdown(s.T().Context()) })
	ctx, span := tp.Tracer("test").Start(s.T().Context(), "test-span")
	defer span.End()
	expectedSC := span.SpanContext()

	s.Run("Info log record carries trace and span IDs from context", func() {
		klogutil.FromContext(ctx).V(1).Info("traced-info", "k", "v")
		klog.Flush()

		records := allRecords(recorder.Result())
		s.Require().NotEmpty(records, "expected at least one OTel record")

		recordSC := trace.SpanContextFromContext(records[0].Context)
		s.True(recordSC.HasTraceID(), "OTel log record must carry a trace ID")
		s.Equal(expectedSC.TraceID(), recordSC.TraceID(), "trace ID must match the span in context")
		s.Equal(expectedSC.SpanID(), recordSC.SpanID(), "span ID must match the span in context")
	})

	recorder.Reset()

	s.Run("Error log record carries trace and span IDs from context", func() {
		klogutil.FromContext(ctx).Error(errSentinel, "traced-error", "code", 42)
		klog.Flush()

		records := allRecords(recorder.Result())
		s.Require().NotEmpty(records, "expected at least one OTel record")

		recordSC := trace.SpanContextFromContext(records[0].Context)
		s.True(recordSC.HasTraceID(), "OTel log record must carry a trace ID")
		s.Equal(expectedSC.TraceID(), recordSC.TraceID())
		s.Equal(expectedSC.SpanID(), recordSC.SpanID())
	})
}

func (s *SinkSuite) TestKlogutilFromContextTextIsClean() {
	s.Run("OTel disabled (default): text log carries no ctx field", func() {
		path := filepath.Join(s.tempDir, "disabled.log")
		sink, err := logging.New(&config.StaticConfig{LogLevel: 1, LogFile: path}, s.httpOut, s.errOut)
		s.Require().NoError(err)
		s.T().Cleanup(func() { _ = sink.Close() })

		klogutil.FromContext(s.T().Context()).Info("default-line", "k", "v")
		klog.Flush()

		content, err := os.ReadFile(path)
		s.Require().NoError(err)
		s.NotContains(string(content), "otel-ctx", "text log must not carry the correlation context")
	})

	s.Run("OTel enabled: text log clean, OTel record still correlates", func() {
		recorder := logtest.NewRecorder()
		otelSink := telemetry.NewLogSink("svc", "1.0.0", recorder)
		path := filepath.Join(s.tempDir, "enabled.log")
		sink, err := logging.New(&config.StaticConfig{LogLevel: 1, LogFile: path}, s.httpOut, s.errOut, logging.WithOtelLogSink(otelSink, nil))
		s.Require().NoError(err)
		s.T().Cleanup(func() { _ = sink.Close() })

		tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
		s.T().Cleanup(func() { _ = tp.Shutdown(s.T().Context()) })
		ctx, span := tp.Tracer("test").Start(s.T().Context(), "span")
		defer span.End()

		klogutil.FromContext(ctx).Info("enabled-line", "k", "v")
		klog.Flush()

		content, err := os.ReadFile(path)
		s.Require().NoError(err)
		s.NotContains(string(content), "otel-ctx", "text log must not carry the correlation context")

		var records []logtest.Record
		for _, recs := range recorder.Result() {
			records = append(records, recs...)
		}
		s.Require().NotEmpty(records)
		s.True(trace.SpanContextFromContext(records[0].Context).HasTraceID(), "OTel record must still carry the trace context")
	})
}

func (s *SinkSuite) TestCloseShutdownsOtelProvider() {
	var shutdownCalled atomic.Bool
	provider := &fakeLogProvider{onShutdown: func() { shutdownCalled.Store(true) }}

	recorder := logtest.NewRecorder()
	otelSink := telemetry.NewLogSink("test-svc", "1.0.0", recorder)

	sink, err := logging.New(
		&config.StaticConfig{LogLevel: 1, LogFile: logging.StderrSentinel},
		s.httpOut, s.errOut,
		logging.WithOtelLogSink(otelSink, provider),
	)
	s.Require().NoError(err)

	s.Require().NoError(sink.Close())
	s.True(shutdownCalled.Load(), "Close must call Shutdown on the OTel LogProvider")
}

// --- OTel test helpers ---------------------------------------------------

var errSentinel = errString("sentinel")

type errString string

func (e errString) Error() string { return string(e) }

// fakeLogProvider satisfies logging.LogProvider and records whether
// Shutdown was called.
type fakeLogProvider struct {
	onShutdown func()
}

func (f *fakeLogProvider) Shutdown(_ context.Context) error {
	if f.onShutdown != nil {
		f.onShutdown()
	}
	return nil
}

func allRecords(recording logtest.Recording) []logtest.Record {
	var out []logtest.Record
	for _, recs := range recording {
		out = append(out, recs...)
	}
	return out
}

func hasAttr(r logtest.Record, key attribute.Key, value string) bool {
	for _, a := range r.Attributes {
		if a.Key == key && a.Value.AsString() == value {
			return true
		}
	}
	return false
}

func TestSink(t *testing.T) {
	suite.Run(t, new(SinkSuite))
}
