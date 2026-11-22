// Package controllers reconciles FederatedMCPServer CRs into the tas-mcp
// federation gateway's registry. Each CR describes one downstream MCP server;
// the controller keeps the gateway in sync — registering on create/update,
// unregistering on delete (guarded by a finalizer).
//
// Ownership (Plan #22 C.2): every row the operator writes carries
// managed_by=operator, and the operator never updates or deletes a row whose
// managed_by is anything else (a server configured in AIQG). A CR whose id
// collides with such a row reports phase Failed with Registered=False reason
// ManagedByAIQG, requeues at the resync interval, and leaves
// the row alone, so the operator's drift healing never fights the UI.
package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/Tributary-ai-services/federated-mcp-operator/api/v1"
	"github.com/Tributary-ai-services/federated-mcp-operator/pkg/gateway"
)

const finalizerName = "federatedmcpserver.mcp.tas.ai/finalizer"

// requeueAfterError is how long to wait before retrying when the gateway is
// unreachable — the gateway may simply be rolling.
const requeueAfterError = 30 * time.Second

// DefaultResyncInterval is how often a healthy, unchanged CR is re-checked
// against the gateway to detect drift (the gateway registry is in-memory, so a
// gateway restart drops all registrations and the operator must re-push them).
const DefaultResyncInterval = 2 * time.Minute

// FederatedMCPServerReconciler reconciles a FederatedMCPServer object against a
// tas-mcp gateway's federation registry.
type FederatedMCPServerReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Gateway *gateway.Client

	// ResyncInterval is the drift re-check cadence for healthy CRs. Zero uses
	// DefaultResyncInterval.
	ResyncInterval time.Duration
}

func (r *FederatedMCPServerReconciler) resyncInterval() time.Duration {
	if r.ResyncInterval > 0 {
		return r.ResyncInterval
	}
	return DefaultResyncInterval
}

//+kubebuilder:rbac:groups=mcp.tas.ai,resources=federatedmcpservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mcp.tas.ai,resources=federatedmcpservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mcp.tas.ai,resources=federatedmcpservers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile drives one FederatedMCPServer toward its desired registration state.
func (r *FederatedMCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var fms mcpv1.FederatedMCPServer
	if err := r.Get(ctx, req.NamespacedName, &fms); err != nil {
		// Not found: nothing to do (finalizer handled the cleanup already).
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// --- Deletion path: unregister from the gateway, then drop the finalizer.
	if !fms.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&fms, finalizerName) {
			id := registeredID(&fms)
			if err := r.unregisterOwn(ctx, id); err != nil {
				logger.Error(err, "unregister from gateway failed; will retry", "id", id)
				return ctrl.Result{RequeueAfter: requeueAfterError}, nil
			}
			controllerutil.RemoveFinalizer(&fms, finalizerName)
			if err := r.Update(ctx, &fms); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// --- Ensure finalizer before we create any external state.
	if !controllerutil.ContainsFinalizer(&fms, finalizerName) {
		controllerutil.AddFinalizer(&fms, finalizerName)
		if err := r.Update(ctx, &fms); err != nil {
			return ctrl.Result{}, err
		}
		// Update re-queues; continue on the next pass.
		return ctrl.Result{}, nil
	}

	// --- Build the registration payload (reads the auth Secret if referenced).
	server, err := r.buildServer(ctx, &fms)
	if err != nil {
		return r.fail(ctx, &fms, "BuildPayload", err)
	}

	// --- Spec unchanged since we last registered: verify the gateway STILL has
	// the server rather than trusting our own status. The gateway registry is
	// in-memory, so a gateway restart silently drops it; on that drift we fall
	// through and re-register. Either way we requeue so drift is re-checked
	// periodically even with no CR events.
	specUnchanged := fms.Status.Registered &&
		fms.Status.ObservedGeneration == fms.Generation &&
		fms.Status.RegisteredID == server.ID
	if specUnchanged {
		state, exists, err := r.Gateway.Get(ctx, server.ID)
		if err != nil {
			logger.Error(err, "gateway drift check failed; will retry", "id", server.ID)
			return ctrl.Result{RequeueAfter: requeueAfterError}, nil
		}
		if exists && state.ManagedByOther() {
			return r.conflict(ctx, &fms, server.ID, state.ManagedBy)
		}
		if exists {
			// Registered is not healthy: report what the gateway's probe saw.
			if setReachable(&fms, state, time.Now()) {
				if err := r.Status().Update(ctx, &fms); err != nil {
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{RequeueAfter: r.resyncInterval()}, nil
		}
		logger.Info("gateway is missing a registered server; re-registering (drift)",
			"id", server.ID)
		// fall through to re-register
	}

	// --- If the id changed, unregister the old id first.
	if fms.Status.RegisteredID != "" && fms.Status.RegisteredID != server.ID {
		if err := r.unregisterOwn(ctx, fms.Status.RegisteredID); err != nil {
			return r.fail(ctx, &fms, "UnregisterOld", err)
		}
	}

	// --- Register (idempotent): on a duplicate, re-register to pick up changes.
	if err := r.Gateway.Register(ctx, server); err != nil {
		if errors.Is(err, gateway.ErrAlreadyRegistered) {
			state, _, gerr := r.Gateway.Get(ctx, server.ID)
			if gerr != nil {
				return r.fail(ctx, &fms, "OwnershipCheck", gerr)
			}
			if state.ManagedByOther() {
				return r.conflict(ctx, &fms, server.ID, state.ManagedBy)
			}
			if uerr := r.Gateway.Unregister(ctx, server.ID); uerr != nil {
				return r.fail(ctx, &fms, "ReregisterUnregister", uerr)
			}
			if rerr := r.Gateway.Register(ctx, server); rerr != nil {
				return r.fail(ctx, &fms, "Reregister", rerr)
			}
		} else {
			logger.Error(err, "register failed; will retry")
			return ctrl.Result{RequeueAfter: requeueAfterError}, nil
		}
	}

	return r.markRegistered(ctx, &fms, server.ID)
}

// buildServer maps the CR spec onto the gateway registration payload, resolving
// the auth Secret into an auth config map when referenced.
func (r *FederatedMCPServerReconciler) buildServer(ctx context.Context, fms *mcpv1.FederatedMCPServer) (gateway.Server, error) {
	auth := gateway.Auth{Type: emptyDefault(fms.Spec.Auth.Type, "none")}
	if auth.Type != "none" && fms.Spec.Auth.SecretRef != nil {
		var secret corev1.Secret
		key := types.NamespacedName{Namespace: fms.Namespace, Name: fms.Spec.Auth.SecretRef.Name}
		if err := r.Get(ctx, key, &secret); err != nil {
			return gateway.Server{}, err
		}
		auth.Config = make(map[string]string, len(secret.Data))
		for k, v := range secret.Data {
			auth.Config[k] = string(v)
		}
	}

	return gateway.Server{
		ID:           desiredID(fms),
		Name:         fms.Spec.DisplayName,
		Description:  fms.Spec.Description,
		Version:      emptyDefault(fms.Spec.Version, "1.0.0"),
		Category:     fms.Spec.Category,
		Endpoint:     fms.Spec.Endpoint,
		Protocol:     emptyDefault(fms.Spec.Protocol, "http"),
		Auth:         auth,
		Capabilities: fms.Spec.Capabilities,
		Tags:         fms.Spec.Tags,
		Metadata:     fms.Spec.Metadata,
		Reduce:       fms.Spec.Reduce,
		ManagedBy:    gateway.ManagedByOperator,
	}, nil
}

// unregisterOwn deletes id from the gateway unless the row is managed by
// someone else (AIQG), which it leaves in place. Absent is success.
func (r *FederatedMCPServerReconciler) unregisterOwn(ctx context.Context, id string) error {
	state, exists, err := r.Gateway.Get(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if state.ManagedByOther() {
		log.FromContext(ctx).Info("leaving a gateway row managed elsewhere in place",
			"id", id, "managed_by", state.ManagedBy)
		return nil
	}
	return r.Gateway.Unregister(ctx, id)
}

// conflict reports that the CR's id belongs to a row the operator does not
// manage, and requeues at the resync interval without touching the row.
func (r *FederatedMCPServerReconciler) conflict(ctx context.Context, fms *mcpv1.FederatedMCPServer, id, owner string) (ctrl.Result, error) {
	msg := fmt.Sprintf("gateway server %q is managed_by %q; the operator leaves it alone (rename spec.serverID or remove the row in AIQG)", id, owner)
	log.FromContext(ctx).Info("server id is managed elsewhere; not registering", "id", id, "managed_by", owner)
	// The CRD's phase enum is Pending|Registered|Failed and C.2 makes no CRD
	// change, so the conflict is Failed; the Registered condition's reason
	// (ManagedByAIQG) is what tells it apart from a gateway error.
	fms.Status.Phase = "Failed"
	fms.Status.Registered = false
	fms.Status.ObservedGeneration = fms.Generation
	fms.Status.LastError = msg
	setCondition(fms, "Registered", metav1.ConditionFalse, "ManagedByAIQG", msg)
	setCondition(fms, ConditionReachable, metav1.ConditionUnknown, "NotRegistered",
		"The gateway row for this id is not the operator's")
	if err := r.Status().Update(ctx, fms); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: r.resyncInterval()}, nil
}

func (r *FederatedMCPServerReconciler) markRegistered(ctx context.Context, fms *mcpv1.FederatedMCPServer, id string) (ctrl.Result, error) {
	now := metav1.Now()
	fms.Status.Phase = "Registered"
	fms.Status.Registered = true
	fms.Status.RegisteredID = id
	fms.Status.ObservedGeneration = fms.Generation
	fms.Status.LastRegisteredTime = &now
	fms.Status.LastError = ""
	setCondition(fms, "Registered", metav1.ConditionTrue, "Synced", "Server registered with the gateway")
	// A fresh registration has not been probed yet; the next resync reads
	// the gateway's probe. Until then reachability is Unknown, not assumed.
	setReachable(fms, nil, time.Now())
	if err := r.Status().Update(ctx, fms); err != nil {
		return ctrl.Result{}, err
	}
	// Requeue so the gateway is re-checked for drift even absent CR events.
	return ctrl.Result{RequeueAfter: r.resyncInterval()}, nil
}

func (r *FederatedMCPServerReconciler) fail(ctx context.Context, fms *mcpv1.FederatedMCPServer, reason string, cause error) (ctrl.Result, error) {
	fms.Status.Phase = "Failed"
	fms.Status.Registered = false
	fms.Status.ObservedGeneration = fms.Generation
	fms.Status.LastError = cause.Error()
	setCondition(fms, "Registered", metav1.ConditionFalse, reason, cause.Error())
	setCondition(fms, ConditionReachable, metav1.ConditionUnknown, "NotRegistered",
		"The gateway does not have this server, so nothing probes it")
	if err := r.Status().Update(ctx, fms); err != nil {
		return ctrl.Result{}, err
	}
	// The cause may be transient — most commonly an auth Secret that is created
	// AFTER the CR, or a gateway blip. Nothing else re-triggers this CR (the
	// controller doesn't watch Secrets), so requeue to retry rather than staying
	// Failed until a manual edit.
	return ctrl.Result{RequeueAfter: requeueAfterError}, nil
}

// SetupWithManager wires the reconciler to watch FederatedMCPServer CRs.
func (r *FederatedMCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.FederatedMCPServer{}).
		Complete(r)
}

// --- helpers -----------------------------------------------------------------

func desiredID(fms *mcpv1.FederatedMCPServer) string {
	if fms.Spec.ServerID != "" {
		return fms.Spec.ServerID
	}
	return fms.Name
}

// registeredID returns the id the server is (or was) registered under, for
// unregistration — the recorded status id if present, else the desired id.
func registeredID(fms *mcpv1.FederatedMCPServer) string {
	if fms.Status.RegisteredID != "" {
		return fms.Status.RegisteredID
	}
	return desiredID(fms)
}

func emptyDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// ConditionReachable is true when the gateway's last probe of the server
// (initialize + tools/list, tas-mcp Plan #22 T.4) passed. Registered only
// says the gateway accepted the registration: from 2026-11-03 to 2026-11-19
// every CR said Registered while the gateway reached 1 of its 12 servers
// (MCP-14). Reachable is the condition to read for health.
const ConditionReachable = "Reachable"

// minStaleAfter is the floor for treating a probe result as stale; the
// effective threshold is max(3 x the gateway's probe interval, this).
const minStaleAfter = 3 * time.Minute

// setReachable sets the Reachable condition from the gateway's probe of the
// server and reports whether the condition changed (so the caller writes
// status only on a change, not on every resync). Messages carry no timings,
// so an unchanged state never rewrites the condition.
func setReachable(fms *mcpv1.FederatedMCPServer, st *gateway.State, now time.Time) bool {
	status, reason, msg := reachability(st, now)
	before := findCondition(fms, ConditionReachable)
	if before != nil && before.Status == status && before.Reason == reason &&
		before.Message == msg && before.ObservedGeneration == fms.Generation {
		return false
	}
	setCondition(fms, ConditionReachable, status, reason, msg)
	return true
}

func reachability(st *gateway.State, now time.Time) (status metav1.ConditionStatus, reason, msg string) {
	if st == nil || st.Probe == nil {
		return metav1.ConditionUnknown, "ProbeUnavailable",
			"The gateway reports no probe for this server (SERVER_PROBES_ENABLED off, or the first probe is pending)"
	}
	p := st.Probe
	staleAfter := minStaleAfter
	if d := 3 * time.Duration(p.IntervalSeconds) * time.Second; d > staleAfter {
		staleAfter = d
	}
	if now.Sub(p.CheckedAt) > staleAfter {
		return metav1.ConditionUnknown, "ProbeStale",
			fmt.Sprintf("The gateway's last probe is older than %s", staleAfter)
	}
	if p.Up {
		return metav1.ConditionTrue, "ProbeSucceeded",
			fmt.Sprintf("initialize and tools/list passed; %d tools", p.ToolCount)
	}
	return metav1.ConditionFalse, "ProbeFailed",
		fmt.Sprintf("%s failed: %s", emptyDefault(p.Stage, "probe"), p.Error)
}

func findCondition(fms *mcpv1.FederatedMCPServer, condType string) *metav1.Condition {
	for i := range fms.Status.Conditions {
		if fms.Status.Conditions[i].Type == condType {
			return &fms.Status.Conditions[i]
		}
	}
	return nil
}

func setCondition(fms *mcpv1.FederatedMCPServer, condType string, status metav1.ConditionStatus, reason, msg string) {
	cond := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: fms.Generation,
	}
	for i := range fms.Status.Conditions {
		if fms.Status.Conditions[i].Type == condType {
			// Preserve LastTransitionTime if the status didn't change.
			if fms.Status.Conditions[i].Status == status {
				cond.LastTransitionTime = fms.Status.Conditions[i].LastTransitionTime
			}
			fms.Status.Conditions[i] = cond
			return
		}
	}
	fms.Status.Conditions = append(fms.Status.Conditions, cond)
}
