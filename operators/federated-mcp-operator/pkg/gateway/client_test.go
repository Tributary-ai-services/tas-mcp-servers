package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(h http.HandlerFunc) (*Client, *httptest.Server) {
	srv := httptest.NewServer(h)
	return NewClient(srv.URL, 2*time.Second), srv
}

func TestRegister_Created(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/federation/servers" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	})
	defer srv.Close()

	if err := c.Register(context.Background(), Server{ID: "git", Endpoint: "http://x", Protocol: "http"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestRegister_AlreadyExists(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "server git already exists", http.StatusBadRequest)
	})
	defer srv.Close()

	err := c.Register(context.Background(), Server{ID: "git"})
	if !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("want ErrAlreadyRegistered, got %v", err)
	}
}

func TestRegister_BadRequestOther(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid endpoint", http.StatusBadRequest)
	})
	defer srv.Close()

	err := c.Register(context.Background(), Server{ID: "git"})
	if err == nil || errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("want a generic 400 error, got %v", err)
	}
}

func TestUnregister_Idempotent(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusNoContent, http.StatusNotFound} {
		c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/federation/servers/git" {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(code)
		})
		if err := c.Unregister(context.Background(), "git"); err != nil {
			t.Errorf("Unregister with %d should be nil, got %v", code, err)
		}
		srv.Close()
	}
}

// Exists is the drift signal: 200 => present, 404 => dropped (gateway restarted).
func TestExists_DriftSignal(t *testing.T) {
	cases := []struct {
		code int
		want bool
		err  bool
	}{
		{http.StatusOK, true, false},
		{http.StatusNotFound, false, false},
		{http.StatusInternalServerError, false, true},
	}
	for _, tc := range cases {
		c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/api/v1/federation/servers/git" {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(tc.code)
		})
		got, err := c.Exists(context.Background(), "git")
		if tc.err && err == nil {
			t.Errorf("code %d: expected error", tc.code)
		}
		if !tc.err && err != nil {
			t.Errorf("code %d: unexpected error %v", tc.code, err)
		}
		if got != tc.want {
			t.Errorf("code %d: Exists = %v, want %v", tc.code, got, tc.want)
		}
		srv.Close()
	}
}

// Get carries the gateway's probe block (tas-mcp Plan #22 T.4), and a body
// without one (an older gateway, or probes off) leaves Probe nil.
func TestGet_Probe(t *testing.T) {
	bodies := map[string]bool{
		`{"id":"git","probe":{"up":false,"stage":"tools/list","error":"refused","checked_at":"2026-11-21T18:00:00Z","interval_seconds":60}}`: true,
		`{"id":"git"}`: false,
		``:             false,
	}
	for body, hasProbe := range bodies {
		c, srv := newTestClient(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		})
		st, ok, err := c.Get(context.Background(), "git")
		srv.Close()
		if err != nil || !ok || st == nil {
			t.Fatalf("%q: st=%v ok=%v err=%v", body, st, ok, err)
		}
		if (st.Probe != nil) != hasProbe {
			t.Fatalf("%q: probe=%+v, want present=%v", body, st.Probe, hasProbe)
		}
		if hasProbe && (st.Probe.Up || st.Probe.Stage != "tools/list" || st.Probe.IntervalSeconds != 60 || st.Probe.CheckedAt.IsZero()) {
			t.Errorf("probe decoded wrong: %+v", st.Probe)
		}
	}
}
