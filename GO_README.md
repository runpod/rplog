# rplog (go)
This document covers the Go implementation of rplog. For language-independent documentation, see the [overall package documentation](../README.md).


## Usage:
This package provides an ordinary [slog.Logger](https://pkg.go.dev/log/slog) accessible via the `Log()` function. The first call to any function in this package will initialize the logger with the metadata fields described in the [overall package documentation](../README.md).

Either use the provided `rplog.DebugContext`, `rplog.InfoContext`, `rplog.WarnContext`, and `rplog.ErrorContext` functions to log, or access the `slog.Logger` directly via the `rplog.Log()` function. Traces will automatically be added to the log if a `request_id` is present in the context.

```go
Use the provided `DebugContext`, `InfoContext`, `WarnContext`, and `ErrorContext` functions to log, or access the `slog.Logger` directly.

```go