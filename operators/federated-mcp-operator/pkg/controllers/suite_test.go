package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	mcpv1 "github.com/Tributary-ai-services/federated-mcp-operator/api/v1"
	"github.com/Tributary-ai-services/federated-mcp-operator/pkg/gateway"
)

var (
	testEnv    *envtest.Environment
	k8sClient  client.Client
	testScheme = runtime.NewScheme()
	envErr     error
)

func TestMain(m *testing.M) {
	_ = clientgoscheme.AddToScheme(testScheme)
	_ = mcpv1.AddToScheme(testScheme)

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{"../../config/crd"},
		ErrorIfCRDPathMissing: true,
	}

	var cfg *rest.Config
	cfg, envErr = testEnv.Start()
	if envErr == nil {
		k8sClient, envErr = client.New(cfg, client.Options{Scheme: testScheme})
	}

	code := m.Run()

	if testEnv != nil {
		_ = testEnv.Stop()
	}
	os.Exit(code)
}

// requireEnvtest skips a test when the envtest control plane could not start
// (e.g. KUBEBUILDER_ASSETS is not set), so the suite degrades gracefully
// instead of hard-failing where the binaries aren't provisioned.
func requireEnvtest(t *testing.T) {
	t.Helper()
	if envErr != nil {
		t.Skipf("envtest control plane unavailable (set KUBEBUILDER_ASSETS): %v", envErr)
	}
}

// --- fake gateway ------------------------------------------------------------

// fakeGateway is an in-memory stand-in for the tas-mcp federation registry API.
// It implements the three endpoints the operator uses and can simulate a
// gateway restart (which drops the in-memory registry) so drift healing is
// testable.
type fakeGateway struct {
	mu      sync.Mutex
	servers map[string]gateway.Server
	srv     *httptest.Server
}

func newFakeGateway() *fakeGateway {
	f := &fakeGateway{servers: map[string]gateway.Server{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/federation/servers", f.handleCollection)
	mux.HandleFunc("/api/v1/federation/servers/", f.handleItem)
	f.srv = httptest.NewServer(mux)
	return f
}

func (f *fakeGateway) url() string { return f.srv.URL }
func (f *fakeGateway) close()      { f.srv.Close() }

// restart clears the registry, emulating a gateway pod restart losing all
// in-memory registrations.
func (f *fakeGateway) restart() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.servers = map[string]gateway.Server{}
}

func (f *fakeGateway) has(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.servers[id]
	return ok
}

func (f *fakeGateway) get(id string) (gateway.Server, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.servers[id]
	return s, ok
}

func (f *fakeGateway) handleCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var s gateway.Server
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.servers[s.ID]; exists {
		http.Error(w, "server "+s.ID+" already exists", http.StatusBadRequest)
		return
	}
	f.servers[s.ID] = s
	w.WriteHeader(http.StatusCreated)
}

func (f *fakeGateway) handleItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/federation/servers/")
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		if _, ok := f.servers[id]; !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if _, ok := f.servers[id]; !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		delete(f.servers, id)
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- test helpers ------------------------------------------------------------

// newReconciler builds a reconciler wired to the envtest client and a fake
// gateway.
func newReconciler(f *fakeGateway) *FederatedMCPServerReconciler {
	return &FederatedMCPServerReconciler{
		Client:  k8sClient,
		Scheme:  testScheme,
		Gateway: gateway.NewClient(f.url(), 5*time.Second),
	}
}

// reconcileToRegistered drives Reconcile until the CR reaches a steady state
// (finalizer add on pass 1, registration on pass 2). It returns the last
// result. Calling Reconcile directly keeps the test deterministic — no async
// manager loop.
func reconcileToRegistered(t *testing.T, r *FederatedMCPServerReconciler, key types.NamespacedName) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
			t.Fatalf("reconcile pass %d: %v", i, err)
		}
	}
}

func mustCreate(t *testing.T, obj client.Object) {
	t.Helper()
	if err := k8sClient.Create(context.Background(), obj); err != nil {
		t.Fatalf("create %T: %v", obj, err)
	}
}

func getFMS(t *testing.T, key types.NamespacedName) *mcpv1.FederatedMCPServer {
	t.Helper()
	var fms mcpv1.FederatedMCPServer
	if err := k8sClient.Get(context.Background(), key, &fms); err != nil {
		t.Fatalf("get FederatedMCPServer %s: %v", key, err)
	}
	return &fms
}

func keyOf(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: "default", Name: name}
}

func ctrlRequest(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: keyOf(name)}
}

func objectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: "default"}
}
