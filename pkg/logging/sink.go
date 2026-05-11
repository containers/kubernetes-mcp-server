// Package logging owns the server's klog wiring and the runtime-reloadable
// log destination (file, stderr, or discard).
//
// The whole package exists so that runtime reloads — log_file rotations on
// SIGHUP — never call klog.SetLoggerWithOptions. klog's own contract states
// that "modifying the logger is not thread-safe and should be done while no
// other goroutines invoke log calls, usually during program initialization."
// We honor that by configuring klog exactly once in New and routing every
// subsequent reload through an atomic writer pointer that the textlogger
// writes to.
package logging

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync/atomic"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// StderrSentinel is the magic value users put in log_file to route logs to
// stderr without opening a file on disk.
const StderrSentinel = "stderr"

// maxTextloggerVerbosity makes the textlogger pass everything through; klog's
// own -v flag is the real verbosity gate at runtime. See package docs.
const maxTextloggerVerbosity = 9

// writerHolder lets us put an io.Writer (an interface) inside an
// atomic.Pointer without the awkward double indirection of
// atomic.Pointer[io.Writer].
type writerHolder struct{ io.Writer }

// Sink is the runtime-reloadable log destination. It implements io.Writer
// so the textlogger can write through it; reloads atomically swap the
// underlying writer and close the previous file (if any).
//
// Construct with New. Reload on SIGHUP. Close on shutdown.
type Sink struct {
	// httpOut and errOut are the process-level stdout/stderr (or test
	// substitutes). They are the defaults when log_file is unset or set to
	// the "stderr" sentinel respectively.
	httpOut io.Writer
	errOut  io.Writer

	// httpMode pins whether the running process is serving HTTP. It is
	// captured from cfg.Port at New time and never re-read from config.
	// The serve mode (HTTP vs stdio) is decided once at startup and
	// blocks for the process lifetime, so a SIGHUP-reloaded config that
	// flipped Port must not flip the log destination — flipping to
	// httpOut while still serving stdio would corrupt the MCP protocol
	// channel.
	httpMode bool

	// writer is what every Write call routes through. Updated by reload via
	// Store; never mutated in place.
	writer atomic.Pointer[writerHolder]

	// file is the currently-open log file (nil when the destination is
	// httpOut, errOut, or io.Discard). Owned by the Sink — close happens on
	// reload (when replaced) and Close.
	file atomic.Pointer[os.File]

	// lastLogFile is the last cfg.LogFile value applyDestination acted on,
	// used to short-circuit reloads that wouldn't change the destination
	// (an unchanged "" or "stderr"). A real path is never short-circuited
	// — even an unchanged path needs reopen for logrotate compatibility.
	// Only meaningful once applied is true; the zero value of "" would
	// otherwise spuriously match the empty-log-file case on first call.
	lastLogFile string
	applied     bool

	// klogFlags retains the FlagSet bound to klog's globals so we can flip
	// verbosity at runtime via klogFlags.Set("v", ...). klog protects this
	// path with its own mutex.
	klogFlags *flag.FlagSet

	// sdkLogger is captured once, after klog has been wired, and handed to
	// the MCP SDK as its server-activity logger. It routes through klog and
	// therefore through this Sink, so log_file changes follow it.
	sdkLogger *slog.Logger
}

// New configures klog and returns a Sink ready to use.
//
// httpOut is the writer used in HTTP mode when log_file is unset (typically
// os.Stdout). errOut is the writer used when log_file is "stderr"
// (typically os.Stderr). In stdio mode without log_file the sink writes to
// io.Discard so klog stays out of the protocol channel.
//
// New must be called at most once per process (outside of tests, which
// guard with klog.CaptureState/Restore). It mutates klog's package-level
// logger and rebinds a fresh FlagSet to klog's globals; a second New
// would leave the previous Sink's flag bindings dangling and reset
// klog's writer behind its back. Production wires a single Sink in
// cmd.Complete and reuses it for the process lifetime.
//
// On error, the caller does not need to Close — no file is opened.
func New(cfg *config.StaticConfig, httpOut, errOut io.Writer) (*Sink, error) {
	s := &Sink{
		httpOut:  httpOut,
		errOut:   errOut,
		httpMode: cfg.Port != "",
	}

	if err := s.applyDestination(cfg); err != nil {
		return nil, err
	}

	s.klogFlags = flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(s.klogFlags)
	if cfg.LogLevel >= 0 {
		_ = s.klogFlags.Set("v", strconv.Itoa(cfg.LogLevel))
	}

	cfgOpts := []textlogger.ConfigOption{
		textlogger.Output(s),
		textlogger.Verbosity(maxTextloggerVerbosity),
	}
	klog.SetLoggerWithOptions(textlogger.NewLogger(textlogger.NewConfig(cfgOpts...)))

	s.sdkLogger = slog.New(logr.ToSlogHandler(klog.Background()))
	return s, nil
}

// Write implements io.Writer. It routes to whichever writer Reload most
// recently installed.
func (s *Sink) Write(p []byte) (int, error) {
	h := s.writer.Load()
	if h == nil {
		return io.Discard.Write(p)
	}
	return h.Write(p)
}

// SDKLogger is the slog.Logger to hand to the MCP SDK's ServerOptions. It
// follows reloads — log_file changes apply to SDK-internal logs too.
func (s *Sink) SDKLogger() *slog.Logger {
	return s.sdkLogger
}

// Reload re-applies cfg to the running sink: it opens the new log file (if
// any), atomically swaps the writer, closes the previous file, and updates
// klog's verbosity.
//
// Reload is intended to be called by a single goroutine (the SIGHUP
// handler). Concurrent Reload calls would not corrupt klog state — each
// step is individually atomic — but they could leave the sink pointing at
// an already-closed file via a Store/Swap interleaving. The package
// assumes a single reloader.
//
// The returned error reflects only the destination swap (open-file
// failure). On error, the previous destination is preserved unchanged.
// A verbosity-update failure after a successful destination swap is
// logged via klog.Warningf rather than returned, so the caller's
// success/failure decision lines up cleanly with whether logs are now
// landing in the right place. In practice klog.Level.Set never fails on
// strconv.Itoa output, so this is a defensive path.
func (s *Sink) Reload(cfg *config.StaticConfig) error {
	if err := s.applyDestination(cfg); err != nil {
		return err
	}
	if cfg.LogLevel >= 0 {
		// klog protects this with its own mutex; safe under concurrent V() reads.
		if err := s.klogFlags.Set("v", strconv.Itoa(cfg.LogLevel)); err != nil {
			klog.Warningf("logging: failed to update klog verbosity, destination already swapped: %v", err)
		}
	}
	return nil
}

// Close releases the current log file, if any. It is idempotent and
// re-routes the writer to errOut first so that any klog call after Close
// (e.g. an error log emitted by the caller's defer when Close itself
// returns an error) lands somewhere visible instead of being swallowed by
// the file descriptor we are about to close.
func (s *Sink) Close() error {
	s.writer.Store(&writerHolder{s.errOut})
	old := s.file.Swap(nil)
	if old == nil {
		return nil
	}
	return old.Close()
}

// applyDestination resolves cfg.LogFile to a writer (and optionally an open
// file), atomically swaps it in, and closes the previously-held file. It is
// the only path that opens or closes log files — Reload and New both go
// through it.
//
// A reload that wouldn't change the destination is skipped, with one
// exception: a real file path always reopens, even when the path is
// unchanged, so external rotation tools (logrotate's rename-and-reload
// flow, etc.) work as expected.
func (s *Sink) applyDestination(cfg *config.StaticConfig) error {
	// Skip when the destination is constant for the process and unchanged.
	// Real file paths fall through — see godoc above.
	if s.applied && cfg.LogFile == s.lastLogFile && (cfg.LogFile == "" || cfg.LogFile == StderrSentinel) {
		return nil
	}

	var (
		newFile   *os.File
		newWriter io.Writer
	)

	switch cfg.LogFile {
	case "":
		if s.httpMode {
			newWriter = s.httpOut
		} else {
			// stdio mode: stdout is the MCP protocol channel, don't pollute it.
			newWriter = io.Discard
		}
	case StderrSentinel:
		newWriter = s.errOut
	default:
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return fmt.Errorf("failed to open log file %q: %w", cfg.LogFile, err)
		}
		newFile = f
		newWriter = f
	}

	s.lastLogFile = cfg.LogFile
	s.applied = true
	s.writer.Store(&writerHolder{newWriter})

	// In-flight writes that already loaded the old writer may still hit the
	// previous fd briefly after Close — same race log rotation already
	// accepts (logrotate's copytruncate has the identical property).
	oldFile := s.file.Swap(newFile)
	if oldFile != nil {
		if err := oldFile.Close(); err != nil {
			// Surface this at default verbosity — a Close failure on
			// rotation is the kind of thing that drops fds on disk-pressure
			// or flaky network filesystems.
			klog.Warningf("logging: failed to close previous log file: %v", err)
		}
	}
	return nil
}
