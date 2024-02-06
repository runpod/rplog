package rplog

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/runpod/rplog/metadata"
	"github.com/runpod/rplog/trace"
	"gitlab.com/efronlicht/enve"
)

// slog.Handler implementation that smuggles the Metadata through the slog.Logger.
// It is used to add the metadata to every log record, and it grabs the Trace from the context if it exists.
// Generally speaking, you don't need to use this directly.
type Handler struct {
	slog.Handler
	metadata slog.Attr // pre-marshalled Metadata
}

var (
	once   sync.Once    // guards initialization of the logger
	logger *slog.Logger // cached logger instance. this is usually synonymous with slog.Default, but we cache it here to avoid the overhead of calling slog.Default repeatedly.
)

// Debug logs at LevelDebug with the given context, initializing the logger if necessary.
func Debug(ctx context.Context, msg string, args ...any) {
	Log().DebugContext(ctx, msg, args...)
}

// Info logs at LevelInfo with the given context, initializing the logger if necessary.
func Info(ctx context.Context, msg string, args ...any) { Log().InfoContext(ctx, msg, args...) }

// Warn logs at LevelWarn with the given context, initializing the logger if necessary. This is essentially an alias for Log().WarnContext.
func Warn(ctx context.Context, msg string, args ...any) { Log().WarnContext(ctx, msg, args...) }

// Error logs at LevelError with the given context, initializing the logger if necessary.
func Error(ctx context.Context, msg string, args ...any) {
	Log().ErrorContext(ctx, msg, args...)
}

// LogAttrs is a more efficient version of [.Log] that accepts only Attrs, initializing the logger if necessary.
func LogAttrs(ctx context.Context, lvl slog.Level, msg string, attrs ...slog.Attr) {
	Log().LogAttrs(ctx, lvl, msg, attrs...)
}

// With returns a Logger that includes the given attributes in each output operation. See the documentation for [log/slog.Logger.With].
// Arguments are converted to attributes as if by [Logger.Log].
// It initializes the logger if necessary.
func With(args ...any) *slog.Logger { return Log().With(args...) }

// WithGroup returns a Logger that includes the given group in each output operation. See the documentation for [Logger.WithGroup].
// It initializes the logger if necessary.
func WithGroup(group string) *slog.Logger { return Log().WithGroup(group) }

// Log returns a handle to the initialized logger. All other functions in this package are just wrappers around this one.
// The first call initializes the package: further calls return the same logger.
func Log() *slog.Logger { once.Do(func() { initEager(os.Stderr) }); return logger }

// Initalize the package with one or more writers. This is optional: if you don't call it, the package will initialize itself with a default writer (os.Stderr)
func Init(writers ...io.Writer) {
	once.Do(func() { initEager(writers...) })
}

// eagerly initialize the package. called exactly once by Log.
func initEager(writers ...io.Writer) {
	var w io.Writer
	switch len(writers) {
	case 0:
		panic("rplog.Init: no writers provided")
	case 1:
		w = writers[0]
	default:
		w = io.MultiWriter(writers...)
	}
	jsonHandler := slog.NewJSONHandler(w, &slog.HandlerOptions{AddSource: true, Level: enve.FromTextOr("RUNPOD_LOG_LEVEL", slog.LevelInfo)})
	m := metadata.Get()
	logger = slog.New(&Handler{Handler: jsonHandler, metadata: slog.Group(
		"meta",
		slog.String("checksum", m.Checksum),
		slog.String("commit", m.Commit),
		slog.String("env", m.Env),
		slog.String("instance_id", m.InstanceID),
		slog.String("language_version", m.LanguageVersion),
		slog.String("repo_path", m.RepoPath),
		slog.String("service_start", m.ServiceStart),
		slog.String("service_version", m.ServiceVersion),
		slog.String("service", m.ServiceName),
	)})
	slog.SetDefault(logger)
}

// Handle the log record, adding the metadata to it (always) and the Trace (if it exists).
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if t, ok := trace.FromCtx(ctx); ok {
		now := time.Now()
		traceElapsedMs := now.Sub(t.TraceStart).Milliseconds()
		requestElapsedMs := now.Sub(t.RequestStart).Milliseconds()
		r.AddAttrs(
			h.metadata,
			slog.Group("trace",
				slog.String("trace_id", t.TraceID),
				slog.String("request_id", t.RequestID),
				slog.Int64("trace_elapsed_ms", traceElapsedMs),
				slog.Int64("request_elapsed_ms", requestElapsedMs),
			))
	} else {
		r.AddAttrs(h.metadata)
	}
	return h.Handler.Handle(ctx, r)
}
