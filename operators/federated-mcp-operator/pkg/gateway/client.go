// Package gateway is a thin HTTP client for the tas-mcp federation registry
// API. The operator uses it to register/unregister downstream MCP servers on
// the running gateway's Manager over REST (POST/DELETE
// /api/v1/federation/servers), which is how a CR change becomes a live change
// in the gateway's federation registry.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrAlreadyRegistered is returned by Register when the gateway already has a
// server with the same id. Callers reconcile this by unregister-then-register
// when the desired spec has changed.
var ErrAlreadyRegistered = errors.New("server already registered")

// Server is the registration payload — the subset of the gateway's
// federation.MCPServer that a caller supplies. JSON tags match the gateway's
// decode shape exactly.
type Server struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Version      string            `json:"version,omitempty"`
	Category     string            `json:"category,omitempty"`
	Endpoint     string            `json:"endpoint"`
	Protocol     string            `json:"protocol"`
	Auth         Auth              `json:"auth"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Auth mirrors the gateway's federation.AuthConfig.
type Auth struct {
	Type   string            `json:"type"`
	Config map[string]string `json:"config,omitempty"`
}

// Client talks to a single tas-mcp gateway's federation API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient returns a gateway client for baseURL (e.g.
// "http://prod-tas-mcp-http.tas-mcp-prod.svc.cluster.local:8082").
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

// Register POSTs a server to the gateway registry. It returns
// ErrAlreadyRegistered on a duplicate-id conflict so the caller can decide
// whether to re-register.
func (c *Client) Register(ctx context.Context, s Server) error {
	body, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal server: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/federation/servers", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("register request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		return nil
	case http.StatusBadRequest:
		// The gateway returns 400 for a duplicate id ("already exists"); surface
		// that as a typed error, everything else 400 as a generic failure.
		msg := readBody(resp.Body)
		if isAlreadyExists(msg) {
			return ErrAlreadyRegistered
		}
		return fmt.Errorf("register rejected (400): %s", msg)
	default:
		return fmt.Errorf("register failed: %s: %s", resp.Status, readBody(resp.Body))
	}
}

// Exists reports whether the gateway currently has a server with this id. It is
// the drift signal: the gateway's registry is in-memory, so a gateway restart
// drops every registration, and the operator uses this to detect that a
// previously-registered server is missing and must be re-registered.
func (c *Client) Exists(ctx context.Context, id string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/federation/servers/"+id, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("get request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("get failed: %s: %s", resp.Status, readBody(resp.Body))
	}
}

// Unregister DELETEs a server from the gateway registry. A 404 is treated as
// success (already gone) so the operation is idempotent.
func (c *Client) Unregister(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/api/v1/federation/servers/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("unregister request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return fmt.Errorf("unregister failed: %s: %s", resp.Status, readBody(resp.Body))
	}
}

func readBody(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 4096))
	return string(bytes.TrimSpace(b))
}

func isAlreadyExists(msg string) bool {
	// The gateway's RegisterServer returns an "already exists" / "already
	// registered" style error for a duplicate id.
	return bytes.Contains(bytes.ToLower([]byte(msg)), []byte("already"))
}
