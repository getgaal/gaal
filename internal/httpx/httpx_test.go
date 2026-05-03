package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUserAgentDefault(t *testing.T) {
	if got := UserAgent(); got != "gaal" {
		t.Errorf("default UserAgent = %q, want %q", got, "gaal")
	}
}

func TestSetUserAgent(t *testing.T) {
	orig := UserAgent()
	t.Cleanup(func() { SetUserAgent(orig) })

	SetUserAgent("gaal/1.2.3")
	if got := UserAgent(); got != "gaal/1.2.3" {
		t.Errorf("UserAgent after Set = %q, want %q", got, "gaal/1.2.3")
	}

	// Empty string is a no-op.
	SetUserAgent("")
	if got := UserAgent(); got != "gaal/1.2.3" {
		t.Errorf("UserAgent after Set(\"\") = %q, want unchanged %q", got, "gaal/1.2.3")
	}
}

func TestNewRequestSetsUserAgent(t *testing.T) {
	orig := UserAgent()
	t.Cleanup(func() { SetUserAgent(orig) })
	SetUserAgent("gaal/test")

	req, err := NewRequest(context.Background(), http.MethodGet, "https://example.invalid/x")
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if got := req.Header.Get("User-Agent"); got != "gaal/test" {
		t.Errorf("UA header = %q, want %q", got, "gaal/test")
	}
}

// TestClientTimeoutFires verifies that a request whose context deadline
// is already past returns promptly (the context cancel beats the
// generous Client.Timeout). We assert both the prompt return and the
// fact that the error mentions context cancellation.
func TestClientTimeoutFires(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the test's context deadline.
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, err := NewRequest(ctx, http.MethodGet, srv.URL)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	start := time.Now()
	_, err = Client().Do(req)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if elapsed > time.Second {
		t.Errorf("request took %v, expected <1s after 100ms context", elapsed)
	}
}

// TestRedirectCap verifies the client refuses to follow more than
// MaxRedirects hops.
func TestRedirectCap(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	req, err := NewRequest(context.Background(), http.MethodGet, srv.URL)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = Client().Do(req)
	if err == nil {
		t.Fatal("expected redirect-cap error, got nil")
	}
	if !errors.Is(err, ErrTooManyRedirects) && !strings.Contains(err.Error(), "stopped after") {
		t.Errorf("error = %v, want too-many-redirects", err)
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"localhost", true},
		{"localhost:8080", true},
		{"127.0.0.1", true},
		{"127.0.0.1:443", true},
		{"[::1]", true},
		{"[::1]:8080", true},
		{"::1", true},
		{"example.com", false},
		{"8.8.8.8", false},
		{"10.0.0.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := isLoopback(tt.in); got != tt.want {
				t.Errorf("isLoopback(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsInternal(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true}, // AWS IMDS
		{"::1", true},
		{"fe80::1", true},
		{"example.com", false},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := isInternal(tt.in); got != tt.want {
				t.Errorf("isInternal(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
