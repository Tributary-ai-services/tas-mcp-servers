package controllers

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcpv1 "github.com/Tributary-ai-services/federated-mcp-operator/api/v1"
	"github.com/Tributary-ai-services/federated-mcp-operator/pkg/gateway"
)

func TestReachability(t *testing.T) {
	now := time.Date(2026, 11, 21, 18, 0, 0, 0, time.UTC)
	fresh := now.Add(-30 * time.Second)
	cases := []struct {
		name   string
		st     *gateway.State
		status metav1.ConditionStatus
		reason string
	}{
		{"no state", nil, metav1.ConditionUnknown, "ProbeUnavailable"},
		{"no probe (old gateway or probes off)", &gateway.State{}, metav1.ConditionUnknown, "ProbeUnavailable"},
		{"up", &gateway.State{Probe: &gateway.Probe{Up: true, CheckedAt: fresh, ToolCount: 6, IntervalSeconds: 60}},
			metav1.ConditionTrue, "ProbeSucceeded"},
		{"down", &gateway.State{Probe: &gateway.Probe{Stage: "tools/list", Error: "connection refused",
			CheckedAt: fresh, IntervalSeconds: 60}}, metav1.ConditionFalse, "ProbeFailed"},
		{"stale up", &gateway.State{Probe: &gateway.Probe{Up: true, CheckedAt: now.Add(-4 * time.Minute), IntervalSeconds: 60}},
			metav1.ConditionUnknown, "ProbeStale"},
		{"slow cadence is not stale", &gateway.State{Probe: &gateway.Probe{Up: true, CheckedAt: now.Add(-10 * time.Minute),
			IntervalSeconds: 300}}, metav1.ConditionTrue, "ProbeSucceeded"},
	}
	for _, tc := range cases {
		status, reason, msg := reachability(tc.st, now)
		if status != tc.status || reason != tc.reason || msg == "" {
			t.Errorf("%s: got %s/%s %q, want %s/%s", tc.name, status, reason, msg, tc.status, tc.reason)
		}
	}
}

// An unchanged probe must not rewrite status on every resync.
func TestSetReachable_ReportsChangeOnly(t *testing.T) {
	now := time.Now()
	fms := &mcpv1.FederatedMCPServer{}
	up := &gateway.State{Probe: &gateway.Probe{Up: true, CheckedAt: now, ToolCount: 3, DurationMS: 12, IntervalSeconds: 60}}
	if !setReachable(fms, up, now) {
		t.Fatal("first set must report a change")
	}
	up2 := &gateway.State{Probe: &gateway.Probe{Up: true, CheckedAt: now.Add(time.Minute), ToolCount: 3, DurationMS: 40, IntervalSeconds: 60}}
	if setReachable(fms, up2, now.Add(time.Minute)) {
		t.Error("same state with a newer probe reported a change")
	}
	down := &gateway.State{Probe: &gateway.Probe{Stage: "tools/list", Error: "refused", CheckedAt: now, IntervalSeconds: 60}}
	if !setReachable(fms, down, now) {
		t.Error("up -> down not reported")
	}
	if c := findCondition(fms, ConditionReachable); c == nil || c.Status != metav1.ConditionFalse {
		t.Errorf("condition = %+v", c)
	}
}

// Registered is not healthy: a registered server the gateway cannot reach
// reads Reachable=False, and recovers to True.
func TestReconcile_ReachableFromGatewayProbe(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newReconciler(f)

	name := "reachable-probe"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec:       mcpv1.FederatedMCPServerSpec{DisplayName: "S", Endpoint: "http://s:1"},
	})
	reconcileToRegistered(t, r, keyOf(name))

	cond := func() *metav1.Condition { return findCondition(getFMS(t, keyOf(name)), ConditionReachable) }
	if c := cond(); c == nil || c.Status != metav1.ConditionUnknown {
		t.Fatalf("before any probe: %+v, want Unknown", c)
	}

	f.setProbe(name, &gateway.Probe{Stage: "tools/list", Error: "dial tcp: connection refused",
		CheckedAt: time.Now(), IntervalSeconds: 60, ConsecutiveFailures: 3})
	if _, err := r.Reconcile(context.Background(), ctrlRequest(name)); err != nil {
		t.Fatal(err)
	}
	fms := getFMS(t, keyOf(name))
	if fms.Status.Phase != "Registered" || !fms.Status.Registered {
		t.Fatalf("registration state changed: %+v", fms.Status)
	}
	if c := cond(); c == nil || c.Status != metav1.ConditionFalse || c.Reason != "ProbeFailed" {
		t.Fatalf("unreachable server: %+v, want False/ProbeFailed", c)
	}

	f.setProbe(name, &gateway.Probe{Up: true, CheckedAt: time.Now(), ToolCount: 4, IntervalSeconds: 60})
	if _, err := r.Reconcile(context.Background(), ctrlRequest(name)); err != nil {
		t.Fatal(err)
	}
	if c := cond(); c == nil || c.Status != metav1.ConditionTrue || c.Reason != "ProbeSucceeded" {
		t.Fatalf("recovered server: %+v, want True", c)
	}
}
