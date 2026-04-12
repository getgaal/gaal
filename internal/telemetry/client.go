package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var plausibleEndpoint = "https://usage.getgaal.com/api/event"

const (
	plausibleDomain = "gaal-lite"
	sendTimeout     = 5 * time.Second
)

// plausiblePayload is the JSON body sent to the Plausible Events API.
type plausiblePayload struct {
	Name   string            `json:"name"`
	URL    string            `json:"url"`
	Domain string            `json:"domain"`
	Props  map[string]string `json:"props,omitempty"`
}

// client sends events to the Plausible Events API.
type client struct {
	endpoint  string // overridable for tests
	userAgent string
}

// send posts a single event payload to the Plausible endpoint.
func (c *client) send(p plausiblePayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}
