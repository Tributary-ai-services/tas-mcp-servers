package controllers

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	mcpv1 "github.com/Tributary-ai-services/federated-mcp-operator/api/v1"
)

func TestReconcile_RegistersOnCreate(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newReconciler(f)

	name := "reg-on-create"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec: mcpv1.FederatedMCPServerSpec{
			DisplayName: "Git MCP",
			Endpoint:    "http://git-mcp:3000",
			Protocol:    "http",
			Category:    "development-tools",
		},
	})
	reconcileToRegistered(t, r, keyOf(name))

	if !f.has(name) {
		t.Fatal("gateway did not receive the registration")
	}
	got, _ := f.get(name)
	if got.Endpoint != "http://git-mcp:3000" || got.Name != "Git MCP" {
		t.Errorf("registered payload wrong: %+v", got)
	}

	fms := getFMS(t, keyOf(name))
	if !fms.Status.Registered || fms.Status.Phase != "Registered" {
		t.Errorf("status not Registered: %+v", fms.Status)
	}
	if fms.Status.RegisteredID != name {
		t.Errorf("RegisteredID = %q, want %q", fms.Status.RegisteredID, name)
	}
}

func TestReconcile_UnregistersOnDelete(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newReconciler(f)
	ctx := context.Background()

	name := "unreg-on-delete"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec:       mcpv1.FederatedMCPServerSpec{DisplayName: "S", Endpoint: "http://s:1"},
	})
	reconcileToRegistered(t, r, keyOf(name))
	if !f.has(name) {
		t.Fatal("precondition: server should be registered")
	}

	// Delete: the finalizer keeps the object until we reconcile the deletion.
	if err := k8sClient.Delete(ctx, getFMS(t, keyOf(name))); err != nil {
		t.Fatalf("delete: %v", err)
	}
	reconcileToRegistered(t, r, keyOf(name)) // drives the deletion path

	if f.has(name) {
		t.Error("server still registered after CR deletion")
	}
	var fms mcpv1.FederatedMCPServer
	err := k8sClient.Get(ctx, keyOf(name), &fms)
	if !apierrors.IsNotFound(err) {
		t.Errorf("CR should be gone after finalizer removed, got err=%v", err)
	}
}

// Drift healing: the gateway loses its in-memory registry (restart); the next
// reconcile must detect the server is missing and re-register it.
func TestReconcile_ReRegistersOnDrift(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newReconciler(f)

	name := "drift-heal"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec:       mcpv1.FederatedMCPServerSpec{DisplayName: "S", Endpoint: "http://s:1"},
	})
	reconcileToRegistered(t, r, keyOf(name))
	if !f.has(name) {
		t.Fatal("precondition: registered")
	}

	f.restart() // gateway drops the in-memory registry
	if f.has(name) {
		t.Fatal("restart should have cleared the registry")
	}

	// Spec is unchanged, so a naive controller would no-op here. The drift check
	// (GET /servers/{id} -> 404) must trigger a re-register.
	if _, err := r.Reconcile(context.Background(), ctrlRequest(name)); err != nil {
		t.Fatalf("reconcile after drift: %v", err)
	}
	if !f.has(name) {
		t.Error("drift not healed: server not re-registered after gateway restart")
	}
}

func TestReconcile_ReRegistersOnSpecChange(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newReconciler(f)
	ctx := context.Background()

	name := "spec-change"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec:       mcpv1.FederatedMCPServerSpec{DisplayName: "S", Endpoint: "http://old:1"},
	})
	reconcileToRegistered(t, r, keyOf(name))

	// Change the endpoint; the gateway already has this id, so the controller
	// must unregister-then-register to apply it.
	fms := getFMS(t, keyOf(name))
	fms.Spec.Endpoint = "http://new:2"
	if err := k8sClient.Update(ctx, fms); err != nil {
		t.Fatalf("update spec: %v", err)
	}
	reconcileToRegistered(t, r, keyOf(name))

	got, ok := f.get(name)
	if !ok || got.Endpoint != "http://new:2" {
		t.Errorf("spec change not applied to gateway: %+v (ok=%v)", got, ok)
	}
}

func TestReconcile_ResolvesAuthSecret(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newReconciler(f)

	name := "auth-secret"
	mustCreate(t, &corev1.Secret{
		ObjectMeta: objectMeta(name + "-auth"),
		Data:       map[string][]byte{"api_key": []byte("s3cr3t")},
	})
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec: mcpv1.FederatedMCPServerSpec{
			DisplayName: "S",
			Endpoint:    "http://s:1",
			Auth: mcpv1.AuthSpec{
				Type:      "api_key",
				SecretRef: &mcpv1.SecretReference{Name: name + "-auth"},
			},
		},
	})
	reconcileToRegistered(t, r, keyOf(name))

	got, ok := f.get(name)
	if !ok {
		t.Fatal("server not registered")
	}
	if got.Auth.Type != "api_key" || got.Auth.Config["api_key"] != "s3cr3t" {
		t.Errorf("auth secret not resolved into registration: %+v", got.Auth)
	}
}

func TestReconcile_FailsWhenAuthSecretMissing(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newReconciler(f)

	name := "missing-secret"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec: mcpv1.FederatedMCPServerSpec{
			DisplayName: "S",
			Endpoint:    "http://s:1",
			Auth: mcpv1.AuthSpec{
				Type:      "api_key",
				SecretRef: &mcpv1.SecretReference{Name: "does-not-exist"},
			},
		},
	})
	reconcileToRegistered(t, r, keyOf(name))

	if f.has(name) {
		t.Error("server should not be registered when its auth secret is missing")
	}
	fms := getFMS(t, keyOf(name))
	if fms.Status.Phase != "Failed" || fms.Status.Registered {
		t.Errorf("status should be Failed/unregistered: %+v", fms.Status)
	}

	// The failure path must requeue, or the CR would stay Failed forever (the
	// controller doesn't watch Secrets).
	res, err := r.Reconcile(context.Background(), ctrlRequest(name))
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter <= 0 {
		t.Error("failure path must requeue so the CR can self-heal")
	}
}

// spec.reduce flows through to the gateway registration payload.
func TestReconcile_ForwardsReduceFlag(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newReconciler(f)

	name := "reduce-flag"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec: mcpv1.FederatedMCPServerSpec{
			DisplayName: "S", Endpoint: "http://s:1", Reduce: true,
		},
	})
	reconcileToRegistered(t, r, keyOf(name))

	got, ok := f.get(name)
	if !ok {
		t.Fatal("server not registered")
	}
	if !got.Reduce {
		t.Error("Reduce=true should be forwarded to the gateway registration")
	}
}

// A CR whose auth Secret is created AFTER it must recover on a later reconcile
// (driven in production by the failure-path requeue), not stay Failed forever.
func TestReconcile_RecoversWhenSecretAppears(t *testing.T) {
	requireEnvtest(t)
	f := newFakeGateway()
	defer f.close()
	r := newReconciler(f)

	name := "secret-recovery"
	mustCreate(t, &mcpv1.FederatedMCPServer{
		ObjectMeta: objectMeta(name),
		Spec: mcpv1.FederatedMCPServerSpec{
			DisplayName: "S",
			Endpoint:    "http://s:1",
			Auth: mcpv1.AuthSpec{
				Type:      "api_key",
				SecretRef: &mcpv1.SecretReference{Name: name + "-auth"},
			},
		},
	})
	reconcileToRegistered(t, r, keyOf(name))
	if f.has(name) {
		t.Fatal("precondition: not registered while secret is missing")
	}

	// The Secret shows up later.
	mustCreate(t, &corev1.Secret{
		ObjectMeta: objectMeta(name + "-auth"),
		Data:       map[string][]byte{"api_key": []byte("s3cr3t")},
	})
	reconcileToRegistered(t, r, keyOf(name))

	got, ok := f.get(name)
	if !ok {
		t.Fatal("server should register once the secret appears")
	}
	if got.Auth.Config["api_key"] != "s3cr3t" {
		t.Errorf("auth not resolved after recovery: %+v", got.Auth)
	}
	if fms := getFMS(t, keyOf(name)); fms.Status.Phase != "Registered" {
		t.Errorf("status should recover to Registered, got %q", fms.Status.Phase)
	}
}
