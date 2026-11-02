# rplog (Go)

This document covers the Go implementation of rplog. For the language-independent
documentation, see the [overall package documentation](./README.md).

`rplog` wraps the standard library's [`log/slog`](https://pkg.go.dev/log/slog) to
give every service a uniform, structured (JSON) logger that:

- stamps build & runtime **metadata** — service, env, VCS commit/tag/time,
  hostname, instance ID, and language version — onto every record;
- automatically attaches **trace/request IDs and elapsed timings** taken from the
  request `context.Context`;
- writes newline-delimited JSON to one or more `io.Writer`s (typically `os.Stderr`).

## Installation

```sh
go get github.com/runpod/rplog
```

## Quick start

Call `rplog.Init` once at startup, then log with the standard `slog` package:
`Init` installs rplog's handler as the `slog` default via `slog.SetDefault`.

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/runpod/rplog"
)

func main() {
	// Pass nil to fill the VCS metadata best-effort from the binary's build
	// info, or supply your own *rplog.Metadata (see "Metadata" below).
	rplog.Init(nil, os.Stderr)

	slog.Info("starting up", slog.Int("port", 8080))
	slog.ErrorContext(context.Background(), "boom", slog.String("reason", "example"))
}
```

`Init` requires at least one writer and **panics** if none are given. Pass several
writers to fan out — they are combined with `io.MultiWriter`.

## Metadata

`Init` takes an optional `*rplog.Metadata`:

- pass `nil` to populate the VCS fields best-effort from `debug.ReadBuildInfo`; or
- generate a fully-populated value at build time with the
  [`buildmeta`](./cmd/README.md) tool and pass it in.

Every record carries these fields (see the
[overall docs](./README.md#overview-logs) for the cross-language contract):
`service`, `env`, `vcs_name`, `vcs_commit`, `vcs_tag`, `vcs_time`, `hostname`,
`instance_id`, `language_version`.

## Log levels

Levels follow `slog`: `DEBUG`, `INFO`, `WARN`, `ERROR`. The minimum level is read
once at `Init` from `RUNPOD_LOG_LEVEL` (default `INFO`).

## Tracing and outbound HTTP

A `Trace` (`trace_id`, `request_id`, their source services, and start times)
travels between services as HTTP headers. Any record logged with a context
carrying a `Trace` automatically gets `trace_id`, `request_id`,
`trace_elapsed_ms`, and `request_elapsed_ms`.

A service needs two pieces: `trace.ServerMiddleware` on inbound requests, and
an [`httpclient`](./httpclient) client for outbound ones. Prefer these over the
lower-level [`trace`](./trace) API (`ClientMiddleware`, `FromHeaderOrNew`,
`CtxWith`, ...); `httpclient` already applies the trace layer in the right
order, and also sets a User-Agent.

### Inbound

```go
mux := http.NewServeMux()
// ... register handlers ...
http.ListenAndServe(":8080", trace.ServerMiddleware(mux))
```

`ServerMiddleware` must be the outermost handler so that every other middleware
sees the trace. Log with the request context so the trace fields appear:

```go
func handler(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "handling request")
}
```

`*gin.Engine` is an `http.Handler`, so gin services wrap the engine the same way:

```go
router := gin.New()
// router.Use(...), routes ...
srv := &http.Server{Addr: ":8080", Handler: trace.ServerMiddleware(router)}
```

In gin handlers, use `c.Request.Context()`, not `c`, as the context. By default
`(*gin.Context).Value` does not fall back to the request context, so
`trace.FromCtx(c)` finds no trace, logs lose the trace fields, and outbound
calls start a new trace. Alternatively, set `router.ContextWithFallback = true`.

### Outbound

Build one client at startup and inject it wherever the service makes requests:

```go
client := httpclient.New("my-service", version) // User-Agent "my-service/v1.2.3"
```

`New` panics if the name or version contains control characters, so a bad
version string fails at startup rather than on every request. For a User-Agent
not of the `name/version` form, use `httpclient.ForUserAgent(ua)`, which
validates the same way.

Each request then carries the context's trace (or a new one) with a fresh
`request_id`, plus that User-Agent unless the request sets its own. Pass
the inbound context so the trace continues:

```go
req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
resp, err := client.Do(req)
```

Every client shares `http.DefaultTransport`'s connection pool. Prefer injecting
the client to wrapping `http.DefaultClient`.

### Adding middleware

Attach your own middleware (logging, metrics, ...) to the client by passing it
as `base` to `httpclient.Transport`. Don't wrap the returned transport, and
don't add `trace.ClientMiddleware` yourself: the trace layer is outermost, so
middleware inside `base` sees the trace headers.

```go
client := &http.Client{
	Timeout:   httpclient.DefaultTimeout,
	Transport: httpclient.Transport(
		httpclient.UserAgent("my-service", version),
		logMiddleware(http.DefaultTransport),
	),
}
```

### Timeouts

The client's `DefaultTimeout` (30s) is a backstop that applies to every request
alongside any context deadline; whichever is shorter wins. Put per-request
deadlines on the request's context. To override the backstop (e.g. for
streaming or long-polling call sites), derive a client with `WithTimeout`; it
shares the original's transport and connection pool:

```go
streamClient := httpclient.WithTimeout(client, 10*time.Minute)
```

### Inbound validation

Incoming header values are treated as untrusted. An `X-Trace-ID` /
`X-Request-ID` that is empty, longer than 200 bytes, or contains any byte
outside `[A-Za-z0-9._-]` is rejected and a fresh id is minted in its place; the
`X-Trace-Source` / `X-Request-Source` names are validated the same way but
capped tighter and dropped (blanked) rather than regenerated when invalid. This
keeps untrusted callers from injecting into or bloating your logs while still
letting a valid non-UUID id propagate unchanged across the hop.

## Environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `RUNPOD_LOG_LEVEL` | Minimum log level (`DEBUG`/`INFO`/`WARN`/`ERROR`). | `INFO` |
| `RUNPOD_SERVICE_NAME` | Service name used as the trace/request source. | `unknown` |

The [`buildmeta`](./cmd/README.md) tool emits the remaining `RUNPOD_*` build
variables for your deployment.

## Design notes: per-line metadata

`Init` keeps each record lean. Only `vcs_commit` — which uniquely identifies the
build — is stamped on every line; the remaining VCS fields (`vcs_name`, which is
essentially always `"git"`; `vcs_tag`; and `vcs_time`, derivable from the commit)
are logged **once** in the `"rplog initialized"` startup record and can be joined
back via `vcs_commit`. `AddSource` is off by default, since a source file/line
block on every record is a large, mostly-redundant per-line cost.

For a representative record this trims ~40% of the line (~195 bytes: ~130 from
dropping the source block, ~65 from the VCS fields). Callers who want richer
per-line context can build their own `slog.Handler` — see "Downstream usage".

`Metadata.Fields()` still returns the **complete** metadata (all VCS fields +
`instance_id`), so exporters that want the full set per event (e.g. Datadog tags)
are unaffected by the per-line trim.

## Downstream usage

How RunPod services consume this package today (useful context if you change the
logging schema):

- **[`runpod/host`](https://github.com/runpod/host)** — the primary consumer. It
  uses `rplog/trace` heavily (client/server middleware, `FromHeaderOrNew`) and
  embeds `rplog.Metadata` in its logger config. It does **not** call `rplog.Init`;
  instead it builds its own `slog` handler chain (level sampling, a Datadog tee,
  its own `HandlerOptions`). The full VCS metadata reaches Datadog as **tags** via
  `Metadata.Fields()`, while its JSON log lines carry a single `ver` field rather
  than the individual `vcs_*` fields.
- **`runpod/ai-api`** — does **not** use rplog; it has a home-grown logrus+slog
  logger that stamps `service`, `env`, and `version` per line.

Takeaway: both services log a single build/version identifier per line rather than
the full VCS set, which is why `Init` now defaults to `vcs_commit`-only. Because
`host` controls its own handler options, `Init`'s `AddSource` and per-line
defaults affect only callers that use `Init` directly.

## Benchmarks

See [BENCHMARKS.md](./BENCHMARKS.md) for performance numbers and the optimization
analysis.
