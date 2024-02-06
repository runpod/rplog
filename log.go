package rplog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

// DebugContext logs at LevelDebug with the given context, initializing the logger if necessary.
func DebugContext(ctx context.Context, msg string, args ...any) {
	Log().DebugContext(ctx, msg, args...)
}

// InfoContext logs at LevelInfo with the given context, initializing the logger if necessary.
func InfoContext(ctx context.Context, msg string, args ...any) { Log().InfoContext(ctx, msg, args...) }

// WarnContext logs at LevelWarn with the given context, initializing the logger if necessary.
func WarnContext(ctx context.Context, msg string, args ...any) { Log().WarnContext(ctx, msg, args...) }

// ErrorContext logs at LevelError with the given context, initializing the logger if necessary.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Log().ErrorContext(ctx, msg, args...)
}

// LogAttrs is a more efficient version of [.Log] that accepts only Attrs, initializing the logger if necessary.
func LogAttrs(ctx context.Context, lvl slog.Level, msg string, attrs ...slog.Attr) {
	Log().LogAttrs(ctx, lvl, msg, attrs...)
}

// With returns a Logger that includes the given attributes in each output operation. Arguments are converted to attributes as if by [Logger.Log].
// It initializes the logger if necessary.
func With(args ...any) *slog.Logger { return Log().With(args...) }

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

const (
	// The maximum content size per request is 5MB
	maxContentSize = 5 * 1024 * 1024

	// The maximum size for a single log is 256kB
	maxLogSize = 256 * 1024

	// The maximum amount of logs that can be sent in a single request is 1000
	maxLogsPerBatch = 1000

	maxRetries = 5
)

var client = &http.Client{Timeout: 20 * time.Second}

// send a batch of logs to datadog, retrying up to 5 times
func send(buf *bytes.Buffer, apiKey, url string, batch []json.RawMessage) error {
	buf.Reset()
	// write the batch to the buffer as a JSON array
	if err := json.NewEncoder(buf).Encode(batch); err != nil {
		return fmt.Errorf("failed to encode batch: %s", err)
	}
	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %s", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", apiKey)
	var errs []error
	for i := 0; i < maxRetries; i++ {
		time.Sleep(10 * time.Millisecond * time.Duration(i)) // 0 on first iteration, 10ms on second, 20ms on third, etc.
		resp, err := client.Do(req)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			errs = append(errs, fmt.Errorf("failed to send logs: %s", resp.Status))
			continue
		}
		return nil // success! no need to retry

	}
	return fmt.Errorf("failed to send logs after %d retries: %v", maxRetries, errs)
}

// InitDatadog initializes the datadog logger with the given API key. It should be called once at the start of the program.
func InitDatadog(ctx context.Context, apiKey string) {
	once.Do(func() {
		logEvents := make(writer, 1000)
		Init(os.Stderr, logEvents)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		go collectAndSendBatches(ctx, apiKey, logEvents, ticker.C)
	})
}

// collect log entries from the `in` channel and send them to datadog in batches.
func collectAndSendBatches(ctx context.Context, apiKey string, in <-chan json.RawMessage, tick <-chan time.Time) {
	batches := make([]json.RawMessage, 0)
	batchSize := 0
	buf := &bytes.Buffer{}
	url := enve.StringOr("RUNPOD_DATADOG_LOGS_URL", "https://in.logs.betterstack.com")
	for {
		select {
		case <-tick: // Flush the batch every tick
			if len(batches) > 0 {
				send(buf, apiKey, url, batches)
			}
			batches = batches[:0]
		case <-ctx.Done(): // Flush the batch and return
			if len(batches) > 0 {
				send(buf, apiKey, url, batches)
			}
			return
		case entry := <-in: // Collect entries
			// is this entry too large to send / will it make the batch too large?
			if len(batches) >= maxLogsPerBatch || len(entry)+batchSize >= maxContentSize {
				send(buf, apiKey, url, batches)
				batches = batches[:0]
				batchSize = 0
			}
			batches = append(batches, entry)
			batchSize += len(entry)
		}
	}
}

// "Write" a log entry to datadog by sending it to the channel to be read by `collectAndSendBatches`
func (w writer) Write(b []byte) (int, error) {
	if len(b) > maxLogSize {
		return 0, fmt.Errorf("log entry too large: %d bytes > %d bytes", len(b), maxLogSize)
	}
	select {
	case w <- json.RawMessage(b):
		return len(b), nil
	default:
		return 0, fmt.Errorf("failed to write log, channel is full")
	}
}

type writer chan json.RawMessage
