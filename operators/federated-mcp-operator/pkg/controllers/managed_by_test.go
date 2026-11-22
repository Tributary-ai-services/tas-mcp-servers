package controllers

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	mcpv1 "github.com/Tributary-ai-services/federated-mcp-operator/api/v1"
	"github.com/Tributary-ai-services/federated-mcp-operator/pkg/gateway"
)

// Plan #22 C.2: the operator writes managed_by=operator, sends the admin
// token, and never updates or deletes a row managed by AIQG.

const testAdminToken = "operator-admin-token"

func newTokenReconciler(f *fakeGateway) *FederatedMCPServerReconciler {
	f.adminToken = testAdminToken
	r := newReconciler(f)
	r.Gateway = gateway.NewClient(f.url(), 5*time.Second).WithAdminToken(testAdminToken)
	return r
}

func aiqgRow(id string) gateway.Server {
	return gateway.Server{ID: id, Name: "From AIQG", Endpoint: "https://crm.example/mcp", Protocol: "http",
		ManagedBy: "aiqg"}
}

// A CR-managed server is registered with managed_by=operator through a
// gateway that requires the admin token, and re-registered (still the
// operator's) after the gateway loses it.
func TestManagedBy_RegistersAsOperatorAndReRegisters(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newTokenReconciler(f)

	name := "mb-operator"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec:       mcpv1.FederatedMCPServerSpec{DisplayName: "S", Endpoint: "http://s:1"},
	})
	reconcileToRegistered(t, r, keyOf(name))
	got, ok := f.get(name)
	if !ok || got.ManagedBy != gateway.ManagedByOperator {
		t.Fatalf("registered row = %+v (ok=%v), want managed_by=operator", got, ok)
	}

	f.restart() // the row is deleted under the operator
	if _, err := r.Reconcile(context.Background(), ctrlRequest(name)); err != nil {
		t.Fatal(err)
	}
	if got, ok := f.get(name); !ok || got.ManagedBy != gateway.ManagedByOperator {
		t.Fatalf("not re-registered as the operator's: %+v ok=%v", got, ok)
	}
}

// Without the token the token-requiring gateway refuses writes, and the CR is
// not marked Registered.
func TestManagedBy_NoTokenIsRefused(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	f.adminToken = testAdminToken
	r := newReconciler(f) // no token

	name := "mb-no-token"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec:       mcpv1.FederatedMCPServerSpec{DisplayName: "S", Endpoint: "http://s:1"},
	})
	reconcileToRegistered(t, r, keyOf(name))
	if f.has(name) {
		t.Fatal("registered without the admin token")
	}
	if getFMS(t, keyOf(name)).Status.Registered {
		t.Fatal("CR marked Registered although the gateway refused it")
	}
}

// A CR whose id collides with an AIQG row leaves the row untouched, reports
// the conflict, and on deletion does not delete the row.
func TestManagedBy_LeavesAIQGRowUntouched(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newTokenReconciler(f)
	ctx := context.Background()

	name := "mb-collide"
	f.putRow(aiqgRow(name))
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec:       mcpv1.FederatedMCPServerSpec{DisplayName: "CR", Endpoint: "http://cr:1"},
	})
	reconcileToRegistered(t, r, keyOf(name))

	if got, _ := f.get(name); !untouched(got) {
		t.Fatalf("AIQG row changed: %+v", got)
	}
	fms := getFMS(t, keyOf(name))
	if fms.Status.Registered || fms.Status.Phase != "Failed" {
		t.Fatalf("status = %+v, want Failed", fms.Status)
	}
	if c := findCondition(fms, "Registered"); c == nil || c.Reason != "ManagedByAIQG" {
		t.Fatalf("Registered condition = %+v", c)
	}

	if err := k8sClient.Delete(ctx, fms); err != nil {
		t.Fatal(err)
	}
	reconcileToRegistered(t, r, keyOf(name))
	if got, ok := f.get(name); !ok || !untouched(got) {
		t.Fatalf("CR deletion touched the AIQG row: %+v ok=%v", got, ok)
	}
	var gone mcpv1.FederatedMCPServer
	if err := k8sClient.Get(ctx, keyOf(name), &gone); !apierrors.IsNotFound(err) {
		t.Fatalf("CR not finalized: %v", err)
	}
}

// AIQG takes over an id the operator had registered: the drift check sees
// the row is no longer the operator's and stops healing it.
func TestManagedBy_StopsHealingARowAIQGTookOver(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newTokenReconciler(f)

	name := "mb-takeover"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec:       mcpv1.FederatedMCPServerSpec{DisplayName: "CR", Endpoint: "http://cr:1"},
	})
	reconcileToRegistered(t, r, keyOf(name))
	f.putRow(aiqgRow(name))

	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(context.Background(), ctrlRequest(name)); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := f.get(name); !untouched(got) {
		t.Fatalf("operator overwrote the AIQG row: %+v", got)
	}
	fms := getFMS(t, keyOf(name))
	if c := findCondition(fms, "Registered"); fms.Status.Phase != "Failed" || c == nil || c.Reason != "ManagedByAIQG" {
		t.Fatalf("phase = %q condition = %+v, want Failed/ManagedByAIQG", fms.Status.Phase, c)
	}
}

// untouched reports whether a stored row is still the one AIQG wrote.
func untouched(got gateway.Server) bool {
	want := aiqgRow(got.ID)
	return got.Name == want.Name && got.Endpoint == want.Endpoint && got.ManagedBy == want.ManagedBy
}
