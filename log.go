package rplog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/runpod/rplog/trace"
	"gitlab.com/efronlicht/enve"
)

// slog.Handler implementation that smuggles the Metadata through the slog.Logger.
// It is used to add the metadata to every log record, and it grabs the Trace from the context if it exists.
// Generally speaking, you don't need to use this directly.
type Handler struct {
	slog.Handler
}

// see buildmeta.go for the definition of Metadata
type Metadata struct {
	InstanceID, Service, Env            string
	VCSName, VCSCommit, VCSTag, VCSTime string
}

// Initalize the package with one or more writers. This is optional: if you don't call it, the package will initialize itself with a default writer (os.Stderr)
// it's OK to use nil for the metadata: this program will fill in on a best-effort basis.
func Init(m *Metadata, writers ...io.Writer) {
	var w io.Writer
	switch len(writers) {
	case 0:
		panic("rplog.Init: no writers provided")
	case 1:
		w = writers[0]
	default:
		w = io.MultiWriter(writers...)
	}
	if m == nil {
		m = &Metadata{}
		buildinfo, ok := debug.ReadBuildInfo()
		if !ok {
			m.VCSName = "unknown"
			m.VCSCommit = "unknown"
			m.VCSTag = "unknown"
			m.VCSTime = "unknown"
			goto FILLED
		}
		for _, v := range buildinfo.Settings {
			switch v.Key {
			case "vcs":
				m.VCSName = v.Value
			case "vcs.revision", "vcs.commit":
				m.VCSCommit = v.Value
			case "vcs.tag":
				m.VCSTag = v.Value
			case "vcs.time":
				m.VCSTime = v.Value
			}
		}
	}
FILLED:
	fmt.Println("rplog.initEager: found metadata", m)

	jsonHandler := slog.NewJSONHandler(w, &slog.HandlerOptions{AddSource: true, Level: enve.FromTextOr("RUNPOD_LOG_LEVEL", slog.LevelInfo)})

	slog.SetDefault(slog.New(&Handler{Handler: jsonHandler.WithAttrs([]slog.Attr{
		slog.String("vcs_name", m.VCSName),
		slog.String("vcs_commit", m.VCSCommit),
		slog.String("vcs_tag", m.VCSTag),
		slog.String("vcs_time", m.VCSTime),
		slog.String("env", m.Env),
		slog.String("instance_id", m.InstanceID),
		slog.String("service", m.Service),
		slog.String("language_version", runtime.Version()),
	})}))

}

// Handle the log record, adding the metadata to it (always) and the Trace (if it exists).
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if t, ok := trace.FromCtx(ctx); ok {
		now := time.Now()
		traceElapsedMs := now.Sub(t.TraceStart).Milliseconds()
		requestElapsedMs := now.Sub(t.RequestStart).Milliseconds()
		r.AddAttrs(
			slog.String("trace_id", t.TraceID),
			slog.String("request_id", t.RequestID),
			slog.Int64("trace_elapsed_ms", traceElapsedMs),
			slog.Int64("request_elapsed_ms", requestElapsedMs),
		)
	}
	return h.Handler.Handle(ctx, r)
}
