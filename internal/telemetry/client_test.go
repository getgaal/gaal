package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendPageview(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := &client{
		endpoint:  srv.URL,
		userAgent: "gaal-lite/test",
	}

	p := plausiblePayload{
		Name:   "pageview",
		URL:    "app://gaal-lite/cmd/install",
		Domain: plausibleDomain,
		Props:  map[string]string{"version": "1.2.3"},
	}

	if err := c.send(p); err != nil {
		t.Fatalf("send returned error: %v", err)
	}

	// Verify User-Agent header
	if got := capturedReq.Header.Get("User-Agent"); got != "gaal-lite/test" {
		t.Errorf("User-Agent = %q, want %q", got, "gaal-lite/test")
	}

	// Verify Content-Type header
	if got := capturedReq.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	// Verify payload JSON fields
	var got plausiblePayload
	if err := json.Unmarshal(capturedBody, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if got.Name != "pageview" {
		t.Errorf("Name = %q, want %q", got.Name, "pageview")
	}
	if got.URL != "app://gaal-lite/cmd/install" {
		t.Errorf("URL = %q, want %q", got.URL, "app://gaal-lite/cmd/install")
	}
	if got.Domain != plausibleDomain {
		t.Errorf("Domain = %q, want %q", got.Domain, plausibleDomain)
	}
	if got.Props["version"] != "1.2.3" {
		t.Errorf("Props[version] = %q, want %q", got.Props["version"], "1.2.3")
	}
}

func TestSendCustomEvent(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := &client{
		endpoint:  srv.URL,
		userAgent: "gaal-lite/test",
	}

	p := plausiblePayload{
		Name:   "Install",
		URL:    "app://gaal-lite/cmd/install",
		Domain: plausibleDomain,
	}

	if err := c.send(p); err != nil {
		t.Fatalf("send returned error: %v", err)
	}

	var got plausiblePayload
	if err := json.Unmarshal(capturedBody, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if got.Name != "Install" {
		t.Errorf("Name = %q, want %q", got.Name, "Install")
	}
}

func TestSendHandlesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &client{
		endpoint:  srv.URL,
		userAgent: "gaal-lite/test",
	}

	p := plausiblePayload{
		Name:   "pageview",
		URL:    "app://gaal-lite/cmd/install",
		Domain: plausibleDomain,
	}

	err := c.send(p)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
