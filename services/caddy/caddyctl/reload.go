package caddyctl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to the Caddy admin API. server-panel runs on the host beside Caddy,
// so AdminURL is localhost (e.g. http://localhost:2019). It POSTs pre-ADAPTED JSON
// to /load — never a raw Caddyfile — so the admin process never does its own
// blind, folder-reading adapt as the `caddy` user (the outage cause). Satisfies
// reconcile.Reloader.
type Client struct {
	AdminURL string
	HTTP     *http.Client
}

// NewClient returns a Client with a sane default timeout.
func NewClient(adminURL string) *Client {
	return &Client{
		AdminURL: trimSlash(adminURL),
		HTTP:     &http.Client{Timeout: 15 * time.Second},
	}
}

// Load applies adaptedJSON as Caddy's whole config via POST /load. adaptedJSON
// MUST be the output of Adapt (fully-resolved JSON), never a raw Caddyfile.
func (c *Client) Load(ctx context.Context, adaptedJSON []byte) error {
	if len(bytes.TrimSpace(adaptedJSON)) == 0 {
		return fmt.Errorf("load: refusing to POST an empty config")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.AdminURL+"/load", bytes.NewReader(adaptedJSON))
	if err != nil {
		return fmt.Errorf("load: build request: %w", err)
	}
	// application/json makes the admin API load pre-adapted JSON directly — it does
	// NOT re-adapt. (text/caddyfile would make it adapt as the caddy user, which is
	// exactly what we avoid.)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("load: POST %s/load: %w", c.AdminURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("load: admin /load returned HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}

// CurrentConfig fetches Caddy's live config (GET /config/) — the "prior" config
// backed up before each reload for one-command rollback. Returns the raw JSON.
func (c *Client) CurrentConfig(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.AdminURL+"/config/", nil)
	if err != nil {
		return nil, fmt.Errorf("current config: build request: %w", err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("current config: GET %s/config/: %w", c.AdminURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("current config: admin /config/ returned HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
