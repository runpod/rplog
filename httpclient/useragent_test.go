package httpclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserAgent(t *testing.T) {
	tests := []struct {
		name, version, want string
	}{
		{"hapiproxy", "", "hapiproxy"},
		{"hapiproxy", "1.33.0", "hapiproxy/1.33.0"},
		{"other-service", "2.0", "other-service/2.0"},
	}
	for _, tt := range tests {
		if got := UserAgent(tt.name, tt.version); got != tt.want {
			t.Errorf("UserAgent(%q, %q) = %q, want %q", tt.name, tt.version, got, tt.want)
		}
	}
}

var validUserAgentTests = []struct {
	in   string
	want bool
}{
	{"", true},
	{"svc/1.0", true},
	{"svc/1.0 (linux; amd64)", true},
	{"svc/1.0\tx", true},
	{"svc/1.0\r\nX-Injected: evil", false},
	{"svc/1.0\n", false},
	{"svc\x00", false},
	{"svc\x7f", false},
}

func TestValidUserAgent(t *testing.T) {
	for _, tt := range validUserAgentTests {
		if got := ValidUserAgent(tt.in); got != tt.want {
			t.Errorf("ValidUserAgent(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// Every value ValidUserAgent accepts must be sendable, and every value it
// rejects must fail, or the check doesn't match Go's.
func TestValidUserAgentMatchesTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	for _, tt := range validUserAgentTests {
		resp, err := (&http.Client{Transport: Transport(tt.in, nil)}).Get(srv.URL)
		if err == nil {
			resp.Body.Close()
		}
		if (err == nil) != tt.want {
			t.Errorf("UA %q: ValidUserAgent=%v, request err=%v", tt.in, tt.want, err)
		}
	}
}

func TestUserAgentTransport(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.UserAgent()
	}))
	defer srv.Close()

	tests := []struct {
		name, transportUA, requestUA, want string
	}{
		{"default applied", testUA, "", testUA},
		{"caller's own wins", testUA, "caller/1.0", "caller/1.0"},
		{"empty passes through to Go's default", "", "", "Go-http-client/1.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.requestUA != "" {
				req.Header.Set("User-Agent", tt.requestUA)
			}
			client := &http.Client{Transport: userAgentTransport{userAgent: tt.transportUA}}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if got != tt.want {
				t.Errorf("User-Agent = %q, want %q", got, tt.want)
			}
			if h := req.Header.Get("User-Agent"); h != tt.requestUA {
				t.Errorf("request mutated: User-Agent = %q", h)
			}
		})
	}
}

type closeIdleTransport struct {
	http.RoundTripper
	closed bool
}

func (t *closeIdleTransport) CloseIdleConnections() { t.closed = true }

func TestUserAgentTransportCloseIdleConnections(t *testing.T) {
	base := &closeIdleTransport{}
	client := &http.Client{Transport: userAgentTransport{base: base}}
	client.CloseIdleConnections()
	if !base.closed {
		t.Error("CloseIdleConnections was not forwarded to the base transport")
	}
}

type capturingTransport struct {
	closeIdleTransport
	req *http.Request
}

func (c *capturingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.req = r
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

// Middlewares inside base, such as host's logger, rely on seeing trace headers.
func TestTransportWrapsBase(t *testing.T) {
	base := &capturingTransport{}
	client := &http.Client{Transport: Transport(testUA, base)}
	resp, err := client.Get("http://example.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := base.req.Header.Get("X-Trace-ID"); got == "" {
		t.Error("base did not see X-Trace-ID")
	}
	if got := base.req.UserAgent(); got != testUA {
		t.Errorf("User-Agent = %q, want %q", got, testUA)
	}
	client.CloseIdleConnections()
	if !base.closed {
		t.Error("CloseIdleConnections was not forwarded to the base transport")
	}
}
