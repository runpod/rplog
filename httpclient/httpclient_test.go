package httpclient

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

const testUA = "test-service/1.0"

func TestNewSendsUserAgent(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.UserAgent()
	}))
	defer srv.Close()

	c := New(testUA)
	if c.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", c.Timeout, DefaultTimeout)
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != testUA {
		t.Errorf("User-Agent = %q, want %q", got, testUA)
	}

	// New must wrap http.DefaultTransport, never replace it: posthog-go clones
	// it through a *http.Transport type assertion when it builds its client, so
	// a wrapper installed globally panics at startup. That is what took down the
	// first attempt at this change.
	if _, ok := http.DefaultTransport.(*http.Transport); !ok {
		t.Fatalf("http.DefaultTransport = %T, want *http.Transport", http.DefaultTransport)
	}
}

// The derived client must keep the base's transport: a fresh transport per call
// site would give each its own connection pool.
func TestWithTimeoutSharesTransport(t *testing.T) {
	base := New(testUA)
	derived := WithTimeout(base, 4*time.Second)

	if derived == base {
		t.Fatal("WithTimeout returned the base client, so it mutated shared state")
	}
	if derived.Transport != base.Transport {
		t.Errorf("Transport = %v, want the base's %v", derived.Transport, base.Transport)
	}
	if derived.Timeout != 4*time.Second {
		t.Errorf("Timeout = %v, want 4s", derived.Timeout)
	}
	if base.Timeout != DefaultTimeout {
		t.Errorf("base Timeout = %v, want it untouched at %v", base.Timeout, DefaultTimeout)
	}
	if got := WithTimeout(base, 0).Timeout; got != 0 {
		t.Errorf("WithTimeout(base, 0).Timeout = %v, want 0 (streaming call sites rely on it)", got)
	}
}

// The derived client's timeout, not the base's, is the one a request is held
// to — in both directions, including the 0 case.
func TestWithTimeoutOverridesBaseTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(150 * time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	t.Run("shorter than base", func(t *testing.T) {
		base := New(testUA)
		base.Timeout = time.Hour
		resp, err := WithTimeout(base, 20*time.Millisecond).Get(srv.URL)
		if err == nil {
			resp.Body.Close()
			t.Fatal("request succeeded, want timeout from the derived client's 20ms")
		}
		if !os.IsTimeout(err) {
			t.Fatalf("err = %v, want a timeout", err)
		}
	})

	t.Run("longer than base", func(t *testing.T) {
		base := New(testUA)
		base.Timeout = 20 * time.Millisecond
		resp, err := WithTimeout(base, time.Hour).Get(srv.URL)
		if err != nil {
			t.Fatalf("request failed with the base's 20ms still in effect: %v", err)
		}
		resp.Body.Close()
	})

	t.Run("zero disables base timeout", func(t *testing.T) {
		base := New(testUA)
		base.Timeout = 20 * time.Millisecond
		resp, err := WithTimeout(base, 0).Get(srv.URL)
		if err != nil {
			t.Fatalf("request failed with the base's 20ms still in effect: %v", err)
		}
		resp.Body.Close()
	})
}
