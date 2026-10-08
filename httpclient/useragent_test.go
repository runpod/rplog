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
