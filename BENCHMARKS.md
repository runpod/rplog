# Benchmarks & performance analysis

Environment: linux/amd64, Go 1.25.12.
Reproduce with:

```sh
go test -run=^$ -bench=. -benchmem -count=6 ./...
```

## Baseline (before optimization)

| Benchmark | ns/op | B/op | allocs/op |
|---|--:|--:|--:|
| `trace/New` | ~528 | 128 | 4 |
| `trace/newuuid` | ~238 | 64 | 2 |
| `trace/FromHeaderOrNew/valid` | ~645 | 32 | 2 |
| `trace/FromHeaderOrNew/invalid` | ~1231 | 264 | 8 |
| `trace/FromHeaderOrNew/missing` | ~981 | 240 | 7 |
| `trace/SaveToHeader` | ~635 | 136 | 8 |
| `Handler.Handle/with_trace` | ~880 | 0 | 0 |
| `Handler.Handle/no_trace` | ~303 | 0 | 0 |
| `FullLogPath` | ~1385 | 48 | 1 |

## Analysis

- **UUID generation dominates** every ID-minting path. `newuuid` (crypto-random
  UUIDv7 + `.String()`) is ~238ns / 2 allocs and is intrinsic to the `uuid`
  library and to the security guarantee (unpredictable IDs). `New` is exactly
  2× `newuuid` — nothing wasteful to remove. **Left unchanged on purpose.**
- **`Handler.Handle` is already allocation-free** (0 allocs); the `slog` machinery
  in `FullLogPath` accounts for the single 48 B allocation, which is out of our
  control. No action.
- **`FromHeaderOrNew` unconditionally parsed `X-Trace-Start`** even when the header
  was absent — the common first-hop case. The failed `time.Parse("")` allocated a
  `*ParseError` on every such request. This was the one clear, safe win.

## Optimization applied

Skip `time.Parse` when `X-Trace-Start` is empty (`trace/trace.go`,
`FromHeaderOrNew`). Behavior is unchanged (missing/malformed → `now`; future →
clamped + warned); only wasted work on the absent-header path is removed.

`benchstat` (n=6), before → after:

| Benchmark | ns/op | B/op | allocs/op |
|---|--:|--:|--:|
| `FromHeaderOrNew/valid` | −2.8% | ~0% | ~0% |
| `FromHeaderOrNew/invalid` | −8.2% | 264→184 (−30%) | 8→7 |
| `FromHeaderOrNew/missing` | −7.3% | 240→160 (−33%) | 7→6 |

No further optimization was pursued: the remaining cost is crypto-random UUID
generation, which is deliberately not traded away for speed.
