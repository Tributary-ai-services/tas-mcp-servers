package controllers

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	napkinv1 "github.com/Tributary-ai-services/napkin-operator/api/v1"
	minioclient "github.com/Tributary-ai-services/napkin-operator/pkg/minio"
	napkinclient "github.com/Tributary-ai-services/napkin-operator/pkg/napkin"
)

const (
	finalizerName = "napkinvisual.napkin.tas.ai/finalizer"

	phasePending     = "Pending"
	phaseSubmitted   = "Submitted"
	phaseProcessing  = "Processing"
	phaseDownloading = "Downloading"
	phaseUploading   = "Uploading"
	phaseCompleted   = "Completed"
	phaseFailed      = "Failed"
)

// NapkinVisualReconciler reconciles a NapkinVisual object
type NapkinVisualReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	tracer       trace.Tracer
	NapkinURL    string
	MinioClient  *minioclient.Client
}

//+kubebuilder:rbac:groups=napkin.tas.ai,resources=napkinvisuals,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=napkin.tas.ai,resources=napkinvisuals/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=napkin.tas.ai,resources=napkinvisuals/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the main reconciliation logic for NapkinVisual resources
func (r *NapkinVisualReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, span := r.tracer.Start(ctx, "napkinvisual_reconcile")
	defer span.End()

	logger := log.FromContext(ctx)
	span.SetAttributes(
		attribute.String("napkinvisual.name", req.Name),
		attribute.String("napkinvisual.namespace", req.Namespace),
	)

	// Fetch the NapkinVisual instance
	var visual napkinv1.NapkinVisual
	if err := r.Get(ctx, req.NamespacedName, &visual); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NapkinVisual resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		span.RecordError(err)
		logger.Error(err, "Failed to get NapkinVisual")
		return ctrl.Result{}, err
	}

	// Handle finalizer for cleanup
	if visual.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&visual, finalizerName) {
			controllerutil.AddFinalizer(&visual, finalizerName)
			return ctrl.Result{}, r.Update(ctx, &visual)
		}
	} else {
		if controllerutil.ContainsFinalizer(&visual, finalizerName) {
			if err := r.cleanupVisual(ctx, &visual); err != nil {
				span.RecordError(err)
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&visual, finalizerName)
			return ctrl.Result{}, r.Update(ctx, &visual)
		}
		return ctrl.Result{}, nil
	}

	// Set initial status if needed
	if visual.Status.Phase == "" {
		visual.Status.Phase = phasePending
		now := metav1.Now()
		visual.Status.StartTime = &now
		visual.Status.Conditions = []napkinv1.NapkinVisualCondition{
			{
				Type:               "Ready",
				Status:             "False",
				LastTransitionTime: now,
				Reason:             "Initializing",
				Message:            "NapkinVisual is being initialized",
			},
		}
		if err := r.Status().Update(ctx, &visual); err != nil {
			span.RecordError(err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// State machine reconciliation
	switch visual.Status.Phase {
	case phasePending:
		return r.reconcilePending(ctx, &visual)
	case phaseSubmitted, phaseProcessing:
		return r.reconcilePolling(ctx, &visual)
	case phaseDownloading:
		return r.reconcileDownloading(ctx, &visual)
	case phaseUploading:
		return r.reconcileUploading(ctx, &visual)
	case phaseCompleted:
		return ctrl.Result{}, nil
	case phaseFailed:
		// Auto-retry after 5 minutes if retries < 3
		if visual.Status.RetryCount < 3 {
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}
		return ctrl.Result{}, nil
	default:
		logger.Info("Unknown phase, resetting to Pending", "phase", visual.Status.Phase)
		visual.Status.Phase = phasePending
		r.Status().Update(ctx, &visual)
		return ctrl.Result{Requeue: true}, nil
	}
}

// reconcilePending reads the API key and submits the visual generation request
func (r *NapkinVisualReconciler) reconcilePending(ctx context.Context, visual *napkinv1.NapkinVisual) (ctrl.Result, error) {
	ctx, span := r.tracer.Start(ctx, "reconcile_pending")
	defer span.End()
	logger := log.FromContext(ctx)

	// Read API key from Secret
	apiKey, err := r.getAPIKey(ctx, visual)
	if err != nil {
		r.setFailedStatus(ctx, visual, fmt.Sprintf("Failed to read API key: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Create Napkin client and submit
	napkin := napkinclient.NewClient(r.NapkinURL, apiKey)
	resp, err := napkin.Submit(ctx, &napkinclient.SubmitRequest{
		Content:    visual.Spec.Content,
		Format:     visual.Spec.Format,
		StyleId:    visual.Spec.Style.StyleId,
		ColorMode:  visual.Spec.Style.ColorMode,
		Language:   visual.Spec.Language,
		Variations: visual.Spec.Variations,
		Context:    visual.Spec.Context,
	})
	if err != nil {
		logger.Error(err, "Failed to submit visual generation")
		r.setFailedStatus(ctx, visual, fmt.Sprintf("Failed to submit: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	visual.Status.Phase = phaseSubmitted
	visual.Status.NapkinRequestId = resp.ID
	r.Status().Update(ctx, visual)

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// reconcilePolling polls the Napkin API for status
func (r *NapkinVisualReconciler) reconcilePolling(ctx context.Context, visual *napkinv1.NapkinVisual) (ctrl.Result, error) {
	ctx, span := r.tracer.Start(ctx, "reconcile_polling")
	defer span.End()
	logger := log.FromContext(ctx)

	apiKey, err := r.getAPIKey(ctx, visual)
	if err != nil {
		r.setFailedStatus(ctx, visual, fmt.Sprintf("Failed to read API key: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	napkin := napkinclient.NewClient(r.NapkinURL, apiKey)
	status, err := napkin.GetStatus(ctx, visual.Status.NapkinRequestId)
	if err != nil {
		logger.Error(err, "Failed to get visual status")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	switch status.Status {
	case "completed":
		// Store file info and transition to downloading
		var files []napkinv1.GeneratedFileStatus
		for _, f := range status.Files {
			files = append(files, napkinv1.GeneratedFileStatus{
				Index:     f.Index,
				Format:    f.Format,
				ColorMode: f.ColorMode,
				NapkinUrl: f.URL,
				SizeBytes: f.SizeBytes,
			})
		}
		visual.Status.GeneratedFiles = files
		visual.Status.Phase = phaseDownloading
		r.Status().Update(ctx, visual)
		return ctrl.Result{Requeue: true}, nil

	case "failed":
		r.setFailedStatus(ctx, visual, fmt.Sprintf("Napkin generation failed: %s", status.Error))
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil

	case "processing":
		visual.Status.Phase = phaseProcessing
		r.Status().Update(ctx, visual)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil

	default:
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
}

// reconcileDownloading downloads files from Napkin URLs
func (r *NapkinVisualReconciler) reconcileDownloading(ctx context.Context, visual *napkinv1.NapkinVisual) (ctrl.Result, error) {
	ctx, span := r.tracer.Start(ctx, "reconcile_downloading")
	defer span.End()
	logger := log.FromContext(ctx)

	apiKey, err := r.getAPIKey(ctx, visual)
	if err != nil {
		r.setFailedStatus(ctx, visual, fmt.Sprintf("Failed to read API key: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	napkin := napkinclient.NewClient(r.NapkinURL, apiKey)

	// Download all files and transition to uploading
	for i, file := range visual.Status.GeneratedFiles {
		if file.NapkinUrl == "" {
			continue
		}
		data, err := napkin.DownloadFile(ctx, file.NapkinUrl)
		if err != nil {
			logger.Error(err, "Failed to download file", "index", file.Index)
			r.setFailedStatus(ctx, visual, fmt.Sprintf("Failed to download file %d: %v", file.Index, err))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// Upload to MinIO
		bucket := visual.Spec.Storage.Bucket
		if bucket == "" {
			bucket = "napkin-visuals"
		}
		prefix := visual.Spec.Storage.Prefix
		tenantId := visual.Spec.TenantId
		if tenantId == "" {
			tenantId = "default"
		}

		key := fmt.Sprintf("%s%s/%s/%d.%s", prefix, tenantId, visual.Name, file.Index, file.Format)
		contentType := getContentType(file.Format)

		url, err := r.MinioClient.Upload(ctx, bucket, key, data, contentType)
		if err != nil {
			logger.Error(err, "Failed to upload to MinIO", "key", key)
			r.setFailedStatus(ctx, visual, fmt.Sprintf("Failed to upload file %d to MinIO: %v", file.Index, err))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		visual.Status.GeneratedFiles[i].MinioKey = key
		visual.Status.GeneratedFiles[i].MinioUrl = url
		visual.Status.GeneratedFiles[i].SizeBytes = int64(len(data))
	}

	// All files uploaded, mark completed
	now := metav1.Now()
	visual.Status.Phase = phaseCompleted
	visual.Status.CompletionTime = &now
	visual.Status.Conditions = []napkinv1.NapkinVisualCondition{
		{
			Type:               "Ready",
			Status:             "True",
			LastTransitionTime: now,
			Reason:             "Completed",
			Message:            "All visuals generated and stored in MinIO",
		},
	}
	visual.Status.ObservedGeneration = visual.Generation
	r.Status().Update(ctx, visual)

	return ctrl.Result{}, nil
}

// reconcileUploading handles the uploading phase (used if download and upload are separated)
func (r *NapkinVisualReconciler) reconcileUploading(ctx context.Context, visual *napkinv1.NapkinVisual) (ctrl.Result, error) {
	// In this implementation, download and upload happen together in reconcileDownloading
	visual.Status.Phase = phaseCompleted
	r.Status().Update(ctx, visual)
	return ctrl.Result{}, nil
}

// getAPIKey reads the Napkin API key from a referenced Kubernetes Secret
func (r *NapkinVisualReconciler) getAPIKey(ctx context.Context, visual *napkinv1.NapkinVisual) (string, error) {
	secretName := visual.Spec.ApiKeySecretRef.Name
	if secretName == "" {
		secretName = "napkin-api-secret"
	}
	secretKey := visual.Spec.ApiKeySecretRef.Key
	if secretKey == "" {
		secretKey = "NAPKIN_API_KEY"
	}

	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: visual.Namespace,
	}, &secret); err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", secretName, err)
	}

	value, ok := secret.Data[secretKey]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s", secretKey, secretName)
	}

	return string(value), nil
}

// setFailedStatus sets the visual status to Failed with an error message
func (r *NapkinVisualReconciler) setFailedStatus(ctx context.Context, visual *napkinv1.NapkinVisual, message string) {
	visual.Status.Phase = phaseFailed
	visual.Status.LastError = message
	visual.Status.RetryCount++
	now := metav1.Now()
	visual.Status.Conditions = []napkinv1.NapkinVisualCondition{
		{
			Type:               "Ready",
			Status:             "False",
			LastTransitionTime: now,
			Reason:             "Failed",
			Message:            message,
		},
	}
	r.Status().Update(ctx, visual)
}

// cleanupVisual deletes MinIO objects when the CR is deleted
func (r *NapkinVisualReconciler) cleanupVisual(ctx context.Context, visual *napkinv1.NapkinVisual) error {
	ctx, span := r.tracer.Start(ctx, "cleanup_visual")
	defer span.End()
	logger := log.FromContext(ctx)

	bucket := visual.Spec.Storage.Bucket
	if bucket == "" {
		bucket = "napkin-visuals"
	}

	for _, file := range visual.Status.GeneratedFiles {
		if file.MinioKey != "" {
			if err := r.MinioClient.Delete(ctx, bucket, file.MinioKey); err != nil {
				logger.Error(err, "Failed to delete MinIO object during cleanup", "key", file.MinioKey)
				// Continue cleanup even if individual deletes fail
			}
		}
	}

	return nil
}

// getContentType returns the MIME type for a file format
func getContentType(format string) string {
	switch format {
	case "svg":
		return "image/svg+xml"
	case "png":
		return "image/png"
	case "ppt":
		return "application/vnd.ms-powerpoint"
	default:
		return "application/octet-stream"
	}
}

// SetupWithManager sets up the controller with the Manager
func (r *NapkinVisualReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.tracer = otel.Tracer("napkinvisual-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&napkinv1.NapkinVisual{}).
		Complete(r)
}
