package controllers

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datasourcev1 "github.com/Tributary-ai-services/datasource-operator/api/v1"
	"github.com/Tributary-ai-services/datasource-operator/pkg/bridge"
)

const (
	maxBackoff     = 15 * time.Minute
	initialBackoff = 30 * time.Second
)

// DatasourceConnectionReconciler reconciles a DatasourceConnection object
type DatasourceConnectionReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	BridgeClient *bridge.Client
	Logger       *zap.Logger
}

// +kubebuilder:rbac:groups=datasource.tas.ai,resources=datasourceconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datasource.tas.ai,resources=datasourceconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datasource.tas.ai,resources=datasourceconnections/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles DatasourceConnection lifecycle
func (r *DatasourceConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Logger.With(zap.String("datasourceconnection", req.NamespacedName.String()))

	// Fetch the DatasourceConnection
	var dc datasourcev1.DatasourceConnection
	if err := r.Get(ctx, req.NamespacedName, &dc); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log.Info("Reconciling", zap.String("phase", string(dc.Status.Phase)), zap.String("type", dc.Spec.Type))

	switch dc.Status.Phase {
	case "", datasourcev1.PhasePending:
		return r.reconcilePending(ctx, &dc, log)
	case datasourcev1.PhaseTesting:
		return r.reconcileTesting(ctx, &dc, log)
	case datasourcev1.PhaseConnected:
		return r.reconcileConnected(ctx, &dc, log)
	case datasourcev1.PhaseFailed:
		return r.reconcileFailed(ctx, &dc, log)
	default:
		log.Warn("Unknown phase, resetting to Pending", zap.String("phase", string(dc.Status.Phase)))
		dc.Status.Phase = datasourcev1.PhasePending
		if err := r.Status().Update(ctx, &dc); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
}

// reconcilePending validates the spec and transitions to Testing
func (r *DatasourceConnectionReconciler) reconcilePending(ctx context.Context, dc *datasourcev1.DatasourceConnection, log *zap.Logger) (ctrl.Result, error) {
	// Validate that the credentials secret exists
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Namespace: dc.Namespace,
		Name:      dc.Spec.CredentialsSecretRef.Name,
	}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		log.Error("Credentials secret not found", zap.Error(err), zap.String("secret", secretKey.String()))
		dc.Status.Phase = datasourcev1.PhaseFailed
		dc.Status.Message = fmt.Sprintf("Credentials secret %q not found: %v", secretKey.String(), err)
		dc.Status.ObservedGeneration = dc.Generation
		if updateErr := r.Status().Update(ctx, dc); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: initialBackoff}, nil
	}

	// Validate the required keys exist in the secret
	usernameKey := dc.Spec.CredentialsSecretRef.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := dc.Spec.CredentialsSecretRef.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	if _, ok := secret.Data[usernameKey]; !ok {
		dc.Status.Phase = datasourcev1.PhaseFailed
		dc.Status.Message = fmt.Sprintf("Secret %q missing key %q", secretKey.String(), usernameKey)
		dc.Status.ObservedGeneration = dc.Generation
		_ = r.Status().Update(ctx, dc)
		return ctrl.Result{RequeueAfter: initialBackoff}, nil
	}
	if _, ok := secret.Data[passwordKey]; !ok {
		dc.Status.Phase = datasourcev1.PhaseFailed
		dc.Status.Message = fmt.Sprintf("Secret %q missing key %q", secretKey.String(), passwordKey)
		dc.Status.ObservedGeneration = dc.Generation
		_ = r.Status().Update(ctx, dc)
		return ctrl.Result{RequeueAfter: initialBackoff}, nil
	}

	// Transition to Testing
	dc.Status.Phase = datasourcev1.PhaseTesting
	dc.Status.Message = "Validating connection..."
	dc.Status.ObservedGeneration = dc.Generation
	if err := r.Status().Update(ctx, dc); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// reconcileTesting performs the actual connectivity test via the MCP bridge
func (r *DatasourceConnectionReconciler) reconcileTesting(ctx context.Context, dc *datasourcev1.DatasourceConnection, log *zap.Logger) (ctrl.Result, error) {
	testDef, ok := bridge.TestToolDefs[dc.Spec.Type]
	if !ok {
		dc.Status.Phase = datasourcev1.PhaseFailed
		dc.Status.Message = fmt.Sprintf("No test tool defined for type: %s", dc.Spec.Type)
		_ = r.Status().Update(ctx, dc)
		return ctrl.Result{}, nil
	}

	// Determine bridge endpoint
	bridgeURL := dc.Spec.BridgeEndpoint
	if bridgeURL == "" {
		bridgeURL = bridge.DefaultBridgeEndpoints[dc.Spec.Type]
	}
	if bridgeURL == "" {
		dc.Status.Phase = datasourcev1.PhaseFailed
		dc.Status.Message = fmt.Sprintf("No bridge endpoint for type: %s", dc.Spec.Type)
		_ = r.Status().Update(ctx, dc)
		return ctrl.Result{}, nil
	}

	// Read credentials from secret
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: dc.Namespace, Name: dc.Spec.CredentialsSecretRef.Name}, secret); err != nil {
		dc.Status.Phase = datasourcev1.PhaseFailed
		dc.Status.Message = fmt.Sprintf("Failed to read credentials: %v", err)
		_ = r.Status().Update(ctx, dc)
		return ctrl.Result{RequeueAfter: initialBackoff}, nil
	}

	usernameKey := dc.Spec.CredentialsSecretRef.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := dc.Spec.CredentialsSecretRef.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	// Build params with connection info injected
	params := make(map[string]interface{})
	for k, v := range testDef.Params {
		params[k] = v
	}
	params["_connection"] = map[string]interface{}{
		"host":     dc.Spec.Host,
		"port":     dc.Spec.Port,
		"username": string(secret.Data[usernameKey]),
		"password": string(secret.Data[passwordKey]),
		"database": dc.Spec.Database,
		"protocol": dc.Spec.Protocol,
		"ssl_mode": dc.Spec.SSLMode,
	}

	// Call the test tool
	start := time.Now()
	_, err := r.BridgeClient.CallTool(ctx, bridgeURL, testDef.Tool, params)
	latency := time.Since(start).Milliseconds()

	now := metav1.Now()
	dc.Status.LastTestedAt = &now
	dc.Status.LatencyMs = latency

	if err != nil {
		log.Warn("Connection test failed", zap.Error(err), zap.Int64("latency_ms", latency))
		dc.Status.Phase = datasourcev1.PhaseFailed
		dc.Status.LastTestResult = "failed"
		dc.Status.Message = fmt.Sprintf("Connection test failed: %v", err)
		dc.Status.ObservedGeneration = dc.Generation
		if updateErr := r.Status().Update(ctx, dc); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: initialBackoff}, nil
	}

	log.Info("Connection test succeeded", zap.Int64("latency_ms", latency))
	dc.Status.Phase = datasourcev1.PhaseConnected
	dc.Status.LastTestResult = "success"
	dc.Status.Message = fmt.Sprintf("Connected (latency: %dms)", latency)
	dc.Status.ObservedGeneration = dc.Generation
	if updateErr := r.Status().Update(ctx, dc); updateErr != nil {
		return ctrl.Result{}, updateErr
	}

	// Requeue for periodic health check
	interval := time.Duration(dc.Spec.HealthCheckInterval) * time.Second
	if interval <= 0 {
		interval = 300 * time.Second
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

// reconcileConnected re-tests periodically
func (r *DatasourceConnectionReconciler) reconcileConnected(ctx context.Context, dc *datasourcev1.DatasourceConnection, log *zap.Logger) (ctrl.Result, error) {
	// Check if spec changed
	if dc.Generation != dc.Status.ObservedGeneration {
		dc.Status.Phase = datasourcev1.PhaseTesting
		dc.Status.Message = "Spec changed, re-testing..."
		_ = r.Status().Update(ctx, dc)
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if it's time for periodic re-test
	interval := time.Duration(dc.Spec.HealthCheckInterval) * time.Second
	if interval <= 0 {
		interval = 300 * time.Second
	}

	if dc.Status.LastTestedAt != nil {
		elapsed := time.Since(dc.Status.LastTestedAt.Time)
		if elapsed < interval {
			// Not time yet, requeue for remaining time
			return ctrl.Result{RequeueAfter: interval - elapsed}, nil
		}
	}

	// Time for re-test
	dc.Status.Phase = datasourcev1.PhaseTesting
	dc.Status.Message = "Periodic health check..."
	_ = r.Status().Update(ctx, dc)
	return ctrl.Result{Requeue: true}, nil
}

// reconcileFailed retries with exponential backoff
func (r *DatasourceConnectionReconciler) reconcileFailed(ctx context.Context, dc *datasourcev1.DatasourceConnection, log *zap.Logger) (ctrl.Result, error) {
	// Check if spec changed — if so, retry immediately
	if dc.Generation != dc.Status.ObservedGeneration {
		dc.Status.Phase = datasourcev1.PhasePending
		dc.Status.Message = "Spec changed, retrying..."
		_ = r.Status().Update(ctx, dc)
		return ctrl.Result{Requeue: true}, nil
	}

	// Exponential backoff: double the interval since last test, capped at maxBackoff
	backoff := initialBackoff
	if dc.Status.LastTestedAt != nil {
		elapsed := time.Since(dc.Status.LastTestedAt.Time)
		backoff = elapsed * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		if backoff < initialBackoff {
			backoff = initialBackoff
		}
	}

	// Re-test
	dc.Status.Phase = datasourcev1.PhaseTesting
	dc.Status.Message = "Retrying connection..."
	_ = r.Status().Update(ctx, dc)
	return ctrl.Result{Requeue: true}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *DatasourceConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&datasourcev1.DatasourceConnection{}).
		Complete(r)
}
