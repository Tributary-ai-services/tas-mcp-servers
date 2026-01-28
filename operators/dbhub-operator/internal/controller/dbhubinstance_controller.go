/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbhubv1alpha1 "github.com/Tributary-ai-services/dbhub-operator/api/v1alpha1"
)

const (
	// Resource names
	configMapSuffix = "-config"
	secretSuffix    = "-creds"
	serviceSuffix   = ""

	// Labels
	labelApp       = "app.kubernetes.io/name"
	labelInstance  = "app.kubernetes.io/instance"
	labelComponent = "app.kubernetes.io/component"
	labelManagedBy = "app.kubernetes.io/managed-by"

	// Finalizer
	finalizerName = "dbhub.tas.io/finalizer"
)

// DBHubInstanceReconciler reconciles a DBHubInstance object
type DBHubInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dbhub.tas.io,resources=dbhubinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbhub.tas.io,resources=dbhubinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbhub.tas.io,resources=dbhubinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *DBHubInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling DBHubInstance", "name", req.Name, "namespace", req.Namespace)

	// Fetch the DBHubInstance
	instance := &dbhubv1alpha1.DBHubInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DBHubInstance resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get DBHubInstance")
		return ctrl.Result{}, err
	}

	// Update observed generation
	if instance.Status.ObservedGeneration != instance.Generation {
		instance.Status.ObservedGeneration = instance.Generation
	}

	// Set initial phase if not set
	if instance.Status.Phase == "" {
		instance.Status.Phase = dbhubv1alpha1.DBHubInstancePhasePending
	}

	// Find matching databases
	databases, err := r.findMatchingDatabases(ctx, instance)
	if err != nil {
		logger.Error(err, "Failed to find matching databases")
		r.setInstanceStatus(instance, dbhubv1alpha1.DBHubInstancePhaseFailed,
			fmt.Sprintf("Failed to find databases: %v", err))
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	// Update connected databases list
	connectedDBs := make([]string, 0, len(databases))
	for _, db := range databases {
		connectedDBs = append(connectedDBs, db.Name)
	}
	sort.Strings(connectedDBs)
	instance.Status.ConnectedDatabases = connectedDBs

	// Generate config and credentials
	configData, credentialsData, err := r.generateConfig(ctx, instance, databases)
	if err != nil {
		logger.Error(err, "Failed to generate config")
		r.setInstanceStatus(instance, dbhubv1alpha1.DBHubInstancePhaseFailed,
			fmt.Sprintf("Failed to generate config: %v", err))
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	// Calculate config hash
	configHash := r.calculateHash(configData)
	instance.Status.ConfigHash = configHash

	// Reconcile ConfigMap
	if err := r.reconcileConfigMap(ctx, instance, configData); err != nil {
		logger.Error(err, "Failed to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	// Reconcile credentials Secret
	if err := r.reconcileCredentialsSecret(ctx, instance, credentialsData); err != nil {
		logger.Error(err, "Failed to reconcile credentials Secret")
		return ctrl.Result{}, err
	}

	// Reconcile Deployment
	if err := r.reconcileDeployment(ctx, instance, configHash); err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		return ctrl.Result{}, err
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, instance); err != nil {
		logger.Error(err, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}

	// Update status
	instance.Status.Endpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d",
		instance.Name+serviceSuffix, instance.Namespace, instance.GetPort())

	// Get deployment to check replicas
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}, deployment); err == nil {
		instance.Status.AvailableReplicas = deployment.Status.AvailableReplicas

		if deployment.Status.AvailableReplicas > 0 {
			r.setInstanceStatus(instance, dbhubv1alpha1.DBHubInstancePhaseRunning, "")
		} else if deployment.Status.UnavailableReplicas > 0 {
			r.setInstanceStatus(instance, dbhubv1alpha1.DBHubInstancePhaseDegraded,
				"Some replicas are not available")
		} else {
			r.setInstanceStatus(instance, dbhubv1alpha1.DBHubInstancePhasePending,
				"Waiting for replicas to be ready")
		}
	}

	now := metav1.Now()
	instance.Status.LastConfigUpdate = &now

	if err := r.Status().Update(ctx, instance); err != nil {
		logger.Error(err, "Failed to update DBHubInstance status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled DBHubInstance",
		"databases", len(databases),
		"replicas", instance.Status.AvailableReplicas)

	return ctrl.Result{}, nil
}

// findMatchingDatabases finds Database resources matching the selector
func (r *DBHubInstanceReconciler) findMatchingDatabases(ctx context.Context, instance *dbhubv1alpha1.DBHubInstance) ([]dbhubv1alpha1.Database, error) {
	// List all databases in the same namespace
	databaseList := &dbhubv1alpha1.DatabaseList{}
	if err := r.List(ctx, databaseList, client.InNamespace(instance.Namespace)); err != nil {
		return nil, err
	}

	// Filter by selector
	var matching []dbhubv1alpha1.Database
	for _, db := range databaseList.Items {
		if instance.MatchesDatabase(&db) {
			// Only include connected databases
			if db.Status.Phase == dbhubv1alpha1.DatabasePhaseConnected {
				matching = append(matching, db)
			}
		}
	}

	return matching, nil
}

// generateConfig generates the TOML config and credentials for DBHub
func (r *DBHubInstanceReconciler) generateConfig(ctx context.Context, instance *dbhubv1alpha1.DBHubInstance, databases []dbhubv1alpha1.Database) (string, map[string][]byte, error) {
	var configBuilder strings.Builder
	credentials := make(map[string][]byte)

	// Write sources section
	for _, db := range databases {
		envName := r.envName(db.Name)

		// Get credentials for this database
		username, password, err := r.getDatabaseCredentials(ctx, &db)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get credentials for database %s: %w", db.Name, err)
		}

		// Build DSN and store in credentials
		dsn := r.buildDSN(&db, username, password)
		credentials[envName+"_DSN"] = []byte(dsn)

		// Write source entry (uses environment variable placeholder)
		configBuilder.WriteString(fmt.Sprintf(`[[sources]]
id = "%s"
dsn = "${%s_DSN}"
`, db.Name, envName))

		// Add optional settings
		if db.Spec.ConnectionTimeout > 0 {
			configBuilder.WriteString(fmt.Sprintf("connection_timeout = %d\n", db.Spec.ConnectionTimeout))
		}
		if db.Spec.QueryTimeout > 0 {
			configBuilder.WriteString(fmt.Sprintf("query_timeout = %d\n", db.Spec.QueryTimeout))
		}
		configBuilder.WriteString("\n")
	}

	// Write tools section - each tool must reference a source
	if instance.Spec.DefaultPolicy != nil && len(databases) > 0 {
		policy := instance.Spec.DefaultPolicy

		// Generate tools for each database source
		for _, db := range databases {
			// execute_sql tool for this database
			configBuilder.WriteString("[[tools]]\n")
			configBuilder.WriteString("name = \"execute_sql\"\n")
			configBuilder.WriteString(fmt.Sprintf("source = \"%s\"\n", db.Name))
			if policy.ReadOnly {
				configBuilder.WriteString("readonly = true\n")
			}
			if policy.MaxRows > 0 {
				configBuilder.WriteString(fmt.Sprintf("max_rows = %d\n", policy.MaxRows))
			}
			configBuilder.WriteString("\n")

			// search_objects tool for this database
			configBuilder.WriteString("[[tools]]\n")
			configBuilder.WriteString("name = \"search_objects\"\n")
			configBuilder.WriteString(fmt.Sprintf("source = \"%s\"\n", db.Name))
			configBuilder.WriteString("\n")
		}
	}

	return configBuilder.String(), credentials, nil
}

// getDatabaseCredentials fetches credentials from a database's referenced secret
func (r *DBHubInstanceReconciler) getDatabaseCredentials(ctx context.Context, db *dbhubv1alpha1.Database) (string, string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: db.GetCredentialsNamespace(),
		Name:      db.Spec.CredentialsRef.Name,
	}, secret); err != nil {
		return "", "", err
	}

	username := string(secret.Data[db.GetUserKey()])
	password := string(secret.Data[db.GetPasswordKey()])
	return username, password, nil
}

// buildDSN constructs the DSN for a database
func (r *DBHubInstanceReconciler) buildDSN(db *dbhubv1alpha1.Database, username, password string) string {
	spec := db.Spec

	switch spec.Type {
	case dbhubv1alpha1.DatabaseTypePostgres:
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			url.QueryEscape(username),
			url.QueryEscape(password),
			spec.Host,
			spec.Port,
			spec.Database,
			spec.SSLMode,
		)

	case dbhubv1alpha1.DatabaseTypeMySQL, dbhubv1alpha1.DatabaseTypeMariaDB:
		tls := "false"
		if spec.SSLMode == dbhubv1alpha1.SSLModeRequire || spec.SSLMode == dbhubv1alpha1.SSLModeVerifyCA || spec.SSLMode == dbhubv1alpha1.SSLModeVerifyFull {
			tls = "true"
		}
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=%s",
			username, password, spec.Host, spec.Port, spec.Database, tls)

	case dbhubv1alpha1.DatabaseTypeSQLServer:
		return fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
			url.QueryEscape(username),
			url.QueryEscape(password),
			spec.Host,
			spec.Port,
			spec.Database,
		)

	case dbhubv1alpha1.DatabaseTypeSQLite:
		return spec.Database

	default:
		return ""
	}
}

// envName converts a database name to an environment variable name
func (r *DBHubInstanceReconciler) envName(name string) string {
	// Convert to uppercase and replace invalid chars with underscores
	result := strings.ToUpper(name)
	result = strings.ReplaceAll(result, "-", "_")
	result = strings.ReplaceAll(result, ".", "_")
	return result
}

// calculateHash calculates a hash of the config for change detection
func (r *DBHubInstanceReconciler) calculateHash(data string) string {
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h[:8])
}

// reconcileConfigMap creates or updates the ConfigMap with TOML config template
func (r *DBHubInstanceReconciler) reconcileConfigMap(ctx context.Context, instance *dbhubv1alpha1.DBHubInstance, configData string) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + configMapSuffix,
			Namespace: instance.Namespace,
			Labels:    r.labels(instance),
		},
		Data: map[string]string{
			"dbhub.toml": configData,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(instance, configMap, r.Scheme); err != nil {
		return err
	}

	// Create or update
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, configMap)
		}
		return err
	}

	existing.Data = configMap.Data
	existing.Labels = configMap.Labels
	return r.Update(ctx, existing)
}

// reconcileCredentialsSecret creates or updates the credentials Secret
func (r *DBHubInstanceReconciler) reconcileCredentialsSecret(ctx context.Context, instance *dbhubv1alpha1.DBHubInstance, credentials map[string][]byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + secretSuffix,
			Namespace: instance.Namespace,
			Labels:    r.labels(instance),
		},
		Data: credentials,
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(instance, secret, r.Scheme); err != nil {
		return err
	}

	// Create or update
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, secret)
		}
		return err
	}

	existing.Data = secret.Data
	existing.Labels = secret.Labels
	return r.Update(ctx, existing)
}

// reconcileDeployment creates or updates the Deployment
func (r *DBHubInstanceReconciler) reconcileDeployment(ctx context.Context, instance *dbhubv1alpha1.DBHubInstance, configHash string) error {
	replicas := instance.GetReplicas()
	port := instance.GetPort()
	image := instance.GetImage()
	transport := instance.GetTransport()

	labels := r.labels(instance)
	labels["config-hash"] = configHash

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    r.labels(instance),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: r.selectorLabels(instance),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   fmt.Sprintf("%d", port),
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "config-renderer",
							Image: "bhgedigital/envsubst:latest",
							Command: []string{
								"sh", "-c",
								"envsubst < /config-template/dbhub.toml > /config/dbhub.toml",
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: instance.Name + secretSuffix,
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-template",
									MountPath: "/config-template",
								},
								{
									Name:      "config-rendered",
									MountPath: "/config",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "dbhub",
							Image:           image,
							ImagePullPolicy: instance.Spec.ImagePullPolicy,
							Args: []string{
								"--transport", string(transport),
								"--port", fmt.Sprintf("%d", port),
								"--config", "/config/dbhub.toml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: port,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-rendered",
									MountPath: "/config",
									ReadOnly:  true,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt32(port),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt32(port),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config-template",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Name + configMapSuffix,
									},
								},
							},
						},
						{
							Name: "config-rendered",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	// Apply resource requirements if specified
	if instance.Spec.Resources != nil {
		deployment.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
			Requests: instance.Spec.Resources.Requests,
			Limits:   instance.Spec.Resources.Limits,
		}
	}

	// Apply node selector if specified
	if instance.Spec.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}

	// Apply tolerations if specified
	if instance.Spec.Tolerations != nil {
		deployment.Spec.Template.Spec.Tolerations = instance.Spec.Tolerations
	}

	// Apply affinity if specified
	if instance.Spec.Affinity != nil {
		deployment.Spec.Template.Spec.Affinity = instance.Spec.Affinity
	}

	// Apply service account if specified
	if instance.Spec.ServiceAccountName != "" {
		deployment.Spec.Template.Spec.ServiceAccountName = instance.Spec.ServiceAccountName
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(instance, deployment, r.Scheme); err != nil {
		return err
	}

	// Create or update
	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, deployment)
		}
		return err
	}

	existing.Spec = deployment.Spec
	existing.Labels = deployment.Labels
	return r.Update(ctx, existing)
}

// reconcileService creates or updates the Service
func (r *DBHubInstanceReconciler) reconcileService(ctx context.Context, instance *dbhubv1alpha1.DBHubInstance) error {
	port := instance.GetPort()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + serviceSuffix,
			Namespace: instance.Namespace,
			Labels:    r.labels(instance),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: r.selectorLabels(instance),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt32(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
		return err
	}

	// Create or update
	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, service)
		}
		return err
	}

	// Preserve ClusterIP
	service.Spec.ClusterIP = existing.Spec.ClusterIP
	existing.Spec = service.Spec
	existing.Labels = service.Labels
	return r.Update(ctx, existing)
}

// labels returns the common labels for resources
func (r *DBHubInstanceReconciler) labels(instance *dbhubv1alpha1.DBHubInstance) map[string]string {
	return map[string]string{
		labelApp:       "dbhub",
		labelInstance:  instance.Name,
		labelComponent: "database-mcp",
		labelManagedBy: "dbhub-operator",
	}
}

// selectorLabels returns the labels used for pod selection
func (r *DBHubInstanceReconciler) selectorLabels(instance *dbhubv1alpha1.DBHubInstance) map[string]string {
	return map[string]string{
		labelApp:      "dbhub",
		labelInstance: instance.Name,
	}
}

// setInstanceStatus updates the instance status
func (r *DBHubInstanceReconciler) setInstanceStatus(instance *dbhubv1alpha1.DBHubInstance, phase dbhubv1alpha1.DBHubInstancePhase, message string) {
	instance.Status.Phase = phase

	now := metav1.Now()
	if phase == dbhubv1alpha1.DBHubInstancePhaseRunning {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionTrue,
			Reason:             "InstanceRunning",
			Message:            "DBHub instance is running",
			LastTransitionTime: now,
		})
	} else {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             string(phase),
			Message:            message,
			LastTransitionTime: now,
		})
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DBHubInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbhubv1alpha1.DBHubInstance{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Watches(
			&dbhubv1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(r.findInstancesForDatabase),
		).
		Named("dbhubinstance").
		Complete(r)
}

// findInstancesForDatabase finds DBHubInstances that should be reconciled when a Database changes
func (r *DBHubInstanceReconciler) findInstancesForDatabase(ctx context.Context, obj client.Object) []reconcile.Request {
	db := obj.(*dbhubv1alpha1.Database)

	// List all DBHubInstances in the same namespace
	instanceList := &dbhubv1alpha1.DBHubInstanceList{}
	if err := r.List(ctx, instanceList, client.InNamespace(db.Namespace)); err != nil {
		return nil
	}

	// Find instances that match this database
	var requests []reconcile.Request
	for _, instance := range instanceList.Items {
		if instance.MatchesDatabase(db) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				},
			})
		}
	}

	return requests
}
