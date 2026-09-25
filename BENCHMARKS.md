# Benchmarks & performance analysis

Environment: linux/amd64, AMD Ryzen Threadripper PRO 3945WX, Go 1.27.1.
Numbers are the median of `-count=6`. Reproduce with:

```sh
go test -run=^$ -bench=. -benchmem -count=6 ./...
```

## Results

`github.com/runpod/rplog`:

| Benchmark | ns/op | B/op | allocs/op |
|---|--:|--:|--:|
| `HandlerHandle/with_trace` | ~1675 | 0 | 0 |
| `HandlerHandle/no_trace` | ~695 | 0 | 0 |
| `FullLogPath` | ~2690 | 48 | 1 |

`github.com/runpod/rplog/trace`:

| Benchmark | ns/op | B/op | allocs/op |
|---|--:|--:|--:|
| `New` | ~1035 | 128 | 4 |
| `NewUUID` | ~495 | 64 | 2 |
| `ValidID` | ~74 | 0 | 0 |
| `FromHeaderOrNew/present` | ~940 | 32 | 2 |
| `FromHeaderOrNew/absent` | ~1580 | 160 | 6 |
| `FromHeaderOrNew/invalid` | ~2175 | 160 | 6 |
| `SaveToHeader` | ~1465 | 136 | 8 |
| `ClientMiddlewareRoundTrip` | ~3905 | 872 | 15 |

## Analysis

- **UUID generation dominates every id-minting path.** `NewUUID` (crypto-random
  UUIDv7 + `.String()`) is ~495ns / 2 allocs and is intrinsic to the `uuid`
  library and to the security guarantee (unpredictable ids). `New` mints two ids,
  so it is ~2× `NewUUID` — nothing wasteful to remove.
- **A valid inbound trace is the cheap path.** `FromHeaderOrNew/present` reuses
  the caller's already-valid ids (2 allocs), while `/absent` and `/invalid` must
  regenerate both ids (6 allocs) — the extra cost is the two fresh UUIDs, not the
  validation. `ValidID` scans a header value in ~74ns and allocates nothing.
- **`FromHeaderOrNew` skips `time.Parse` when `X-Trace-Start` is absent** (the
  common first-hop case), avoiding a `*ParseError` allocation on every such
  request. Behavior is unchanged: absent/malformed → `now`; a future timestamp is
  clamped to `now` silently (no per-request warn, so a caller cannot drive log
  volume through the header).
- **`HandlerHandle` is allocation-free** (0 allocs); the single 48 B allocation in
  `FullLogPath` comes from the `slog` machinery, which is out of our control.
