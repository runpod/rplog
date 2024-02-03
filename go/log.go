package rplog

import (
	"context"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"
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

// get program metadata by reading environment variables and debug.BuildInfo
func getMetadata() Metadata {
	buildinfo, _ := debug.ReadBuildInfo()
	if buildinfo == nil { // just replace the nil with an empty struct: we'll fill in the defaults later
		buildinfo = &debug.BuildInfo{}
	}
	commit := "unknown"
	for _, v := range buildinfo.Settings {
		if v.Key == "vcs.commit" {
			commit = v.Value
			break
		}
	}
	instanceID, err := uuid.NewV7()
	if err != nil {
		instanceID = uuid.New()
	}

	return Metadata{
		Checksum:        buildinfo.Main.Sum,
		Commit:          commit,
		Env:             enve.StringOr("ENV", "dev"),
		InstanceID:      instanceID,
		ServiceVersion:  enve.StringOr("RUNPOD_SERVICE_NAME", "unknown"),
		LanguageVersion: or(buildinfo.GoVersion, "unknown"),
		RepoPath:        or(buildinfo.Main.Path, "unknown"),
		ServiceStart:    time.Now().UTC().Format(time.RFC3339),
		Version:         buildinfo.Main.Version,
	}
}

var (
	once   sync.Once
	logger *slog.Logger
)

// Logger returns a handle to the initialized logger.
// The first call initializes the package: further calls return the same logger.
func Logger() *slog.Logger {
	once.Do(initEager)
	return logger
}

func initEager() {
	jsonHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{AddSource: true, Level: enve.FromTextOr("RUNPOD_LOG_LEVEL", slog.LevelInfo)})
	m := getMetadata()
	logger = slog.New(&Handler{Handler: jsonHandler, metadata: slog.Group(
		"meta",
		slog.String("checksum", m.Checksum),
		slog.String("commit", m.Commit),
		slog.String("env", m.Env),
		slog.String("instance_id", m.InstanceID.String()),
		slog.String("language_version", m.LanguageVersion),
		slog.String("repo_path", m.RepoPath),
		slog.String("service_start", m.ServiceStart),
		slog.String("service", m.ServiceVersion),
		slog.String("version", m.Version),
	)})
	slog.SetDefault(logger)
}

// see https://www.notion.so/runpod/log-meeting-1-31-2024-03a18a6d6ab84b16b5806e81493cc72d for the design.
// metadata is collected once per service start and is immutable.
type Metadata struct{ Checksum, Commit, Env, InstanceID, LanguageVersion, RepoPath, ServiceStart, ServiceVersion, Version string }

// Handle the log record, adding the metadata to it (always) and the Trace (if it exists).
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if t, ok := trace.FromCtx(ctx); ok {
		r.AddAttrs(h.metadata, slog.String("trace_id", t.TraceID), slog.String("request_id", t.RequestID))
	} else {
		r.AddAttrs(h.metadata)
	}
	return h.Handler.Handle(ctx, r)
}

// return a if it's non-zero, otherwise b.
func or[T comparable](a, b T) T {
	var zero T
	if a == zero {
		return b
	}
	return a
}
