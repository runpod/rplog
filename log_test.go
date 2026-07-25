package rplog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/runpod/rplog/trace"
)

// newTestHandler builds a Handler writing JSON to buf, bypassing Init's global
// slog.SetDefault so tests stay isolated and race-safe.
func newTestHandler(buf io.Writer) *Handler {
	return &Handler{Handler: slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})}
}

func logAndParse(t *testing.T, ctx context.Context, msg string, attrs ...slog.Attr) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	h := newTestHandler(&buf)
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
	rec.AddAttrs(attrs...)
	if err := h.Handle(ctx, rec); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output is not valid JSON: %q", buf.String())
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestMetadataFields(t *testing.T) {
	m := &Metadata{
		InstanceID: "id", Service: "svc", Env: "prod",
		VCSName: "git", VCSCommit: "abc", VCSTag: "v1", VCSTime: "2026-01-01T00:00:00Z",
	}
	got := m.Fields()
	want := map[string]any{
		"instance_id": "id", "service": "svc", "env": "prod",
		"vcs_name": "git", "vcs_commit": "abc", "vcs_tag": "v1", "vcs_time": "2026-01-01T00:00:00Z",
	}
	if len(got) != len(want) {
		t.Fatalf("Fields() len = %d, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Fields()[%q] = %v, want %v", k, got[k], v)
		}
	}
}

func TestHandlerHandle(t *testing.T) {
	t.Run("positive/trace attrs added when trace in context", func(t *testing.T) {
		ctx := trace.CtxWith(context.Background(), trace.New())
		m := logAndParse(t, ctx, "hi")
		for _, k := range []string{"trace_id", "request_id", "trace_elapsed_ms", "request_elapsed_ms"} {
			if _, ok := m[k]; !ok {
				t.Errorf("missing %q in output: %v", k, m)
			}
		}
	})

	t.Run("negative/no trace attrs without trace in context", func(t *testing.T) {
		m := logAndParse(t, context.Background(), "hi")
		for _, k := range []string{"trace_id", "request_id", "trace_elapsed_ms", "request_elapsed_ms"} {
			if _, ok := m[k]; ok {
				t.Errorf("unexpected %q in output without trace: %v", k, m)
			}
		}
	})

	t.Run("boundary/future trace start clamps elapsed to zero", func(t *testing.T) {
		future := trace.Trace{
			TraceID:      "t", RequestID: "r",
			TraceStart:   time.Now().Add(time.Hour),
			RequestStart: time.Now().Add(time.Hour),
		}
		ctx := trace.CtxWith(context.Background(), future)
		m := logAndParse(t, ctx, "hi")
		if v := m["trace_elapsed_ms"].(float64); v < 0 {
			t.Errorf("trace_elapsed_ms should be clamped to >= 0, got %v", v)
		}
		if v := m["request_elapsed_ms"].(float64); v < 0 {
			t.Errorf("request_elapsed_ms should be clamped to >= 0, got %v", v)
		}
	})

	t.Run("security/hostile attr values are escaped not injected", func(t *testing.T) {
		var buf bytes.Buffer
		h := newTestHandler(&buf)
		rec := slog.NewRecord(time.Now(), slog.LevelInfo, "evil\n{\"fake\":\"log\"}", 0)
		rec.AddAttrs(slog.String("user", "a\r\nb\tc"))
		if err := h.Handle(context.Background(), rec); err != nil {
			t.Fatal(err)
		}
		// Exactly one JSON object => no injected extra log lines.
		if n := strings.Count(strings.TrimSpace(buf.String()), "\n"); n != 0 {
			t.Errorf("output contains %d embedded newlines; injection possible: %q", n, buf.String())
		}
		if !json.Valid(buf.Bytes()) {
			t.Errorf("hostile input broke JSON validity: %q", buf.String())
		}
	})
}

func TestInit(t *testing.T) {
	// Init mutates global slog state, so these run sequentially (not parallel).

	t.Run("negative/zero writers panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Init with no writers should panic")
			}
		}()
		Init(nil)
	})

	t.Run("positive/single writer receives logs", func(t *testing.T) {
		var buf bytes.Buffer
		Init(&Metadata{Service: "svc", Env: "test"}, &buf)
		slog.Info("hello")
		if !strings.Contains(buf.String(), "hello") {
			t.Errorf("log not written to buffer: %q", buf.String())
		}
		if !strings.Contains(buf.String(), `"service":"svc"`) {
			t.Errorf("metadata not stamped: %q", buf.String())
		}
	})

	t.Run("positive/multiple writers all receive logs", func(t *testing.T) {
		var a, b bytes.Buffer
		Init(&Metadata{Service: "multi"}, &a, &b)
		slog.Info("fanout")
		if !strings.Contains(a.String(), "fanout") || !strings.Contains(b.String(), "fanout") {
			t.Errorf("multiwriter fanout failed: a=%q b=%q", a.String(), b.String())
		}
	})

	t.Run("corner/nil metadata fills best-effort", func(t *testing.T) {
		var buf bytes.Buffer
		Init(nil, &buf)
		slog.Info("defaults")
		if !strings.Contains(buf.String(), "vcs_name") {
			t.Errorf("expected best-effort vcs metadata, got %q", buf.String())
		}
	})
}

// --- Race test (run with -race) ---

// lockedBuffer is a concurrency-safe io.Writer for the race test.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func TestConcurrentHandle(t *testing.T) {
	h := newTestHandler(&lockedBuffer{})
	logger := slog.New(h)
	ctx := trace.CtxWith(context.Background(), trace.New())

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			for range 200 {
				logger.InfoContext(ctx, "concurrent", slog.Int("g", i))
			}
		})
	}
	wg.Wait()
}

// --- Benchmarks ---

func BenchmarkHandlerHandle(b *testing.B) {
	withTrace := trace.CtxWith(context.Background(), trace.New())
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "bench", 0)

	b.Run("with_trace", func(b *testing.B) {
		h := newTestHandler(io.Discard)
		b.ReportAllocs()
		for b.Loop() {
			_ = h.Handle(withTrace, rec)
		}
	})
	b.Run("no_trace", func(b *testing.B) {
		h := newTestHandler(io.Discard)
		b.ReportAllocs()
		for b.Loop() {
			_ = h.Handle(context.Background(), rec)
		}
	})
}

func BenchmarkFullLogPath(b *testing.B) {
	h := newTestHandler(io.Discard)
	logger := slog.New(h)
	ctx := trace.CtxWith(context.Background(), trace.New())
	b.ReportAllocs()
	for b.Loop() {
		logger.InfoContext(ctx, "request handled", slog.Int("status", 200))
	}
}
