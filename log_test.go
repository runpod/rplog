package rplog

import (
	"log/slog"
	"os"
	"testing"
)

func TestLog(t *testing.T) {
	Init(nil, os.Stderr)
	slog.Error("hi")
}
