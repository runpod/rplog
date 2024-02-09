package rplog

import (
	"context"
	"testing"
)

func TestLog(t *testing.T) {
	Error(context.TODO(), "hi")
}
