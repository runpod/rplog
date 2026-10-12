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

func TestUserAgentTransport(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.UserAgent()
	}))
	defer srv.Close()

	get := func(t *testing.T, client *http.Client, ua string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if ua != "" {
			req.Header.Set("User-Agent", ua)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	client := &http.Client{Transport: userAgentTransport{userAgent: "test-service/1.0"}}

	get(t, client, "")
	if got != "test-service/1.0" {
		t.Errorf("User-Agent = %q, want test-service/1.0", got)
	}

	// A caller's own User-Agent wins, and the request it handed us is not mutated.
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "caller/1.0")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != "caller/1.0" {
		t.Errorf("User-Agent = %q, want caller/1.0", got)
	}
	if h := req.Header.Get("User-Agent"); h != "caller/1.0" {
		t.Errorf("request mutated: User-Agent = %q", h)
	}

	// No identity configured: pass through and leave Go's default.
	get(t, &http.Client{Transport: userAgentTransport{}}, "")
	if got != "Go-http-client/1.1" {
		t.Errorf("User-Agent = %q with an empty userAgent, want Go's default", got)
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
