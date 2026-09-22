# rplog — X-Trace-ID hardening plan (trace package)

## Context

`rplog/trace` is the shared library that defines RunPod's cross-service trace contract
(`X-Trace-ID` / `X-Request-ID` + `X-Trace-Start` / `X-Trace-Source` / `X-Request-Source`).
The `host` daemon already depends on it; `hapi` and `proxy` are adopting it, and the RunPod
GraphQL API (TypeScript) is being taught to emit and propagate the same headers. As part of that
end-to-end tracing work, the library needed:

1. **Inbound-header validation.** `FromHeaderOrNew` trusted the raw `X-Trace-ID` / `X-Request-ID`
   verbatim. A CR/LF-bearing value would flow into a `Trace`, then `SaveToHeader` would re-emit it
   on an outbound request or the response echo — failing the request (`net/http` rejects invalid
   header values) or smuggling a header into a lenient downstream.
2. **A defect the Python port carried** (`py/trace.py`): `Trace.new()` raised `AttributeError`
   (`datetime.datetime.now()` on a directly-imported `datetime`), and `save_to_headers` stored a
   **tuple** for `X-Request-ID` (trailing comma) and omitted `X-Trace-Source` / `X-Trace-Start`.
3. **No tests** for the Go `trace` package.

Scope: Go + Python. A JS `trace` module is a separate follow-up (`js/` has none today).

## Changes (done)

### Go — `trace/trace.go`
- Added `validID` (allocation-free byte scan: non-empty, ≤ `maxIDLen` = 200, charset
  `[A-Za-z0-9._-]`), `resolveID` (regenerate on invalid), `resolveSource` (drop invalid to `""`).
- `FromHeaderOrNew` now resolves the two IDs and both sources through those helpers, so a `Trace`
  can never hold bytes that `SaveToHeader` would unsafely re-emit.
- Guarded the `X-Trace-Start` parse: the common no-header path skips `time.Parse` entirely
  (previously it parsed `""` on every request and fell back).
- Removed the now-unused `orelse` generic.

### Python — `py/trace.py`
- Fixed `Trace.new()` (`datetime.now()`), fixed the `save_to_headers` tuple bug, and made it emit
  the same five headers as Go with a freshly minted `X-Request-ID`.
- Added `_valid_id` / `_resolve_id` / `_resolve_source` mirroring the Go validation.

### Tests
- `trace/trace_test.go` — table-driven (`{description, input, expected}`) covering `validID`,
  `resolveID`, `resolveSource`, `FromHeaderOrNew` (present / absent / poisoned / source-drop),
  trace-start (absent / past / invalid / future-clamp), `SaveToHeader` round-trip + no-unsafe-bytes,
  `ClientMiddleware` (reuse trace id, mint new request id) and `ServerMiddleware`, `New()`
  uniqueness (10k), and `AllocsPerRun` proving `validID` is 0-alloc. Plus benchmarks.
- `py/trace_test.py` — `unittest` table-driven mirror, incl. regression tests for both fixed bugs.

## Benchmark results (baseline; `go test -bench=. -benchmem`)

```
BenchmarkFromHeaderOrNew/present     1057 ns/op    32 B/op    2 allocs/op
BenchmarkFromHeaderOrNew/absent      1899 ns/op   160 B/op    6 allocs/op
BenchmarkFromHeaderOrNew/invalid     2220 ns/op   160 B/op    6 allocs/op
BenchmarkValidID                      128 ns/op     0 B/op    0 allocs/op
BenchmarkNew                         1359 ns/op   128 B/op    4 allocs/op
BenchmarkNewUUID                      492 ns/op    64 B/op    2 allocs/op
BenchmarkSaveToHeader                1694 ns/op   136 B/op    8 allocs/op
BenchmarkClientMiddlewareRoundTrip   3982 ns/op   872 B/op   15 allocs/op
```

Review: `validID` meets the 0-alloc requirement. The guarded `time.Parse` keeps the hot inbound
"present" path at 2 allocs / ~1µs; the 6-alloc paths are the uuid-generation cases (unavoidable).
All costs are negligible against real network I/O — no further tuning is justified.

## Remaining / follow-ups

- Tag a new module version (e.g. `v0.1.2`) and bump `require github.com/runpod/rplog` in
  `host` / `hapi` / `proxy` to pick up validation. Backward compatible — the only visible change is
  that a hostile inbound id is sanitized to a fresh uuid.
- Add a JS `trace` module (`js/trace.ts`) so TS/JS services can converge on the shared library
  (RunPod currently uses its own monorepo `@runpod/rplog` helper).
- ~~Update the top-level README to document the validation and response-echo usage.~~ Done:
  the README `## Tracing` section now carries the HTTP header contract table, the Go
  inbound/outbound/response-echo usage, and the inbound-validation behavior. (GO_README stays
  the minimal slog-logger quickstart; the header contract lives in README.)
