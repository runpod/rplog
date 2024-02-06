// the compat.go file contains functions that are provided for compatibility with the old logrus-based logging.
// in general, you should prefer using the new structured logging functions in this package instead of the functions in this file.
package rplog

import (
	"context"
	"fmt"
	"runtime/debug"
)

// Infof logs at LevelInfo with the given context, initializing the logger if necessary.
//
// Deprecated: prefer using Info directly and adding structured data to the log instead of using this function.
// This function is provided to make it easier to migrate from ourold  logrus-based logging.
func Infof(ctx context.Context, msg string, args ...any) { Info(ctx, fmt.Sprintf(msg, args...)) }

// Debugf logs at LevelDebug as though by fmt.Sprintf, initializing the logger if necessary.
//
// Deprecated: prefer using Debug directly and adding structured data to the log instead of using this function.
// This function is provided to make it easier to migrate from ourold  logrus-based logging.
func Debugf(ctx context.Context, msg string, args ...any) { Debug(ctx, fmt.Sprintf(msg, args...)) }

// Errorf logs at LevelError as though by fmt.Sprintf, initializing the logger if necessary.
//
// Deprecated: prefer using Error directly and adding structured data to the log instead of using this function.
// This function is provided to make it easier to migrate from ourold  logrus-based logging.
func Errorf(ctx context.Context, msg string, args ...any) { Error(ctx, fmt.Sprintf(msg, args...)) }

// Warnf logs at LevelWarn as though by fmt.Sprintf, initializing the logger if necessary.
//
// Deprecated: prefer using Warn directly and adding structured data to the log instead of using this function.
// This function is provided to make it easier to migrate from ourold  logrus-based logging.
func Warnf(ctx context.Context, msg string, args ...any) { Warn(ctx, fmt.Sprintf(msg, args...)) }

// Panic logs at error level and then panics with the given message.
// The stack trace is included in the log.
// Try not to use this function, as it is generally better to return an error instead of panicking.
func Panic(ctx context.Context, msg string) {
	stack := debug.Stack()
	lg := Log()
	lg.ErrorContext(ctx, msg, "stack", string(stack))
	panic(msg)
}

// Panicf logs at error level and then panics with the given message, formatted as though by fmt.Sprintf.
// The stack trace is included in the log.
// Try not to use this function, as it is generally better to return an error instead of panicking.
//
// Deprecated: prefer using Panic directly and adding structured data to the log instead of using this function.
func Panicf(ctx context.Context, msg string, args ...any) { Panic(ctx, fmt.Sprintf(msg, args...)) }
