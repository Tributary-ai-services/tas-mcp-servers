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

// AdminTokenHeader carries tas-mcp's MCP_ADMIN_TOKEN. With MCP_TENANT_SERVERS
// on, the gateway refuses register and delete without it (401/403), and a GET
// without it sees only platform servers (Plan #22 C.1).
const AdminTokenHeader = "TAS-MCP-Admin-Token" //nolint:gosec // a header name

// ManagedByOperator marks a registry row as this operator's (Plan #22 C.2).
// The operator writes it on every row it registers and touches no row that
// carries any other value: those are managed in AIQG.
const ManagedByOperator = "operator"

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
	// Reduce opts this server's tool-call results into gateway reduce-at-source.
	Reduce bool `json:"reduce,omitempty"`
	// ManagedBy is who owns the row; the operator always sends
	// ManagedByOperator.
	ManagedBy string `json:"managed_by,omitempty"`
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
	// adminToken is sent in AdminTokenHeader on every request when set.
	adminToken string
}

// WithAdminToken makes the client send token (MCP_ADMIN_TOKEN) on every
// request. Empty sends nothing, which is what a gateway without
// MCP_TENANT_SERVERS expects.
func (c *Client) WithAdminToken(token string) *Client {
	c.adminToken = token
	return c
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	if c.adminToken != "" {
		req.Header.Set(AdminTokenHeader, c.adminToken)
	}
	return c.http.Do(req)
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

	resp, err := c.do(req)
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

// Probe is the gateway's last reachability probe of a server (tas-mcp Plan #22
// T.4): initialize + tools/list, as a client would. JSON tags match tas-mcp's
// federation.ProbeResult.
type Probe struct {
	Up                  bool       `json:"up"`
	Stage               string     `json:"stage,omitempty"`
	Error               string     `json:"error,omitempty"`
	CheckedAt           time.Time  `json:"checked_at"`
	DurationMS          int64      `json:"duration_ms"`
	LastUpAt            *time.Time `json:"last_up_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	ToolCount           int        `json:"tool_count"`
	IntervalSeconds     int        `json:"interval_seconds"`
}

// State is what GET /api/v1/federation/servers/{id} reports about a server.
type State struct {
	// ManagedBy is the row's owner. Empty from a gateway older than C.1 or
	// with MCP_TENANT_SERVERS off, which means the operator.
	ManagedBy string `json:"managed_by,omitempty"`

	// Probe is nil when the gateway does not probe (SERVER_PROBES_ENABLED
	// off, or a gateway older than T.4) or has not probed the server yet.
	Probe *Probe `json:"probe,omitempty"`
}

// ManagedByOther reports whether the row belongs to someone other than the
// operator (AIQG). An absent value is the operator's: rows written before
// C.1 carry none.
func (s *State) ManagedByOther() bool {
	return s != nil && s.ManagedBy != "" && s.ManagedBy != ManagedByOperator
}

// Exists reports whether the gateway currently has a server with this id. It is
// the drift signal: the gateway's registry is in-memory, so a gateway restart
// drops every registration, and the operator uses this to detect that a
// previously-registered server is missing and must be re-registered.
func (c *Client) Exists(ctx context.Context, id string) (bool, error) {
	_, ok, err := c.Get(ctx, id)
	return ok, err
}

// Get returns the gateway's view of a server: ok=false on 404 (not
// registered), and the server's probe result when the gateway has one. One
// request answers both drift ("is it registered?") and reachability ("does
// it answer?").
func (c *Client) Get(ctx context.Context, id string) (*State, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/federation/servers/"+id, http.NoBody)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, false, fmt.Errorf("get request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		st := &State{}
		b, rerr := io.ReadAll(io.LimitReader(resp.Body, maxGetBody))
		if rerr == nil && len(bytes.TrimSpace(b)) > 0 {
			// A body we cannot read still proves the server is registered;
			// it only leaves reachability unknown.
			_ = json.Unmarshal(b, st)
		}
		return st, true, nil
	case http.StatusNotFound:
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("get failed: %s: %s", resp.Status, readBody(resp.Body))
	}
}

// maxGetBody bounds a GET server response (a server definition is small).
const maxGetBody = 1 << 20

// Unregister DELETEs a server from the gateway registry. A 404 is treated as
// success (already gone) so the operation is idempotent.
func (c *Client) Unregister(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/api/v1/federation/servers/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
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
