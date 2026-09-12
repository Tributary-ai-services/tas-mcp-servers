# DBHub Kubernetes Operator

> **Design Pattern:** [MCP Kubernetes Operator Pattern](./design-patterns/mcp-kubernetes-operator-pattern.md)
> **Status:** Implementation Ready
> **MCP Server:** DBHub (Multi-database SQL)

A Kubernetes operator for managing DBHub MCP server instances with custom resources for databases and multi-tenant deployments.

## Overview

This operator provides:
- **Database CR**: Declarative database connection management with health checks
- **DBHubInstance CR**: Automated DBHub deployment with database discovery
- **Multi-tenant support**: Namespace isolation with label-based database selection
- **Credential management**: Integration with Kubernetes Secrets and External Secrets Operator

## Custom Resource Definitions

### Database CRD

```yaml
# crds/dbhub.tas.io_databases.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: databases.dbhub.tas.io
spec:
  group: dbhub.tas.io
  names:
    kind: Database
    listKind: DatabaseList
    plural: databases
    singular: database
    shortNames:
      - db
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          required: ["spec"]
          properties:
            spec:
              type: object
              required: ["type", "credentialsRef"]
              properties:
                type:
                  type: string
                  enum: ["postgres", "mysql", "mariadb", "sqlserver", "sqlite"]
                host:
                  type: string
                port:
                  type: integer
                database:
                  type: string
                credentialsRef:
                  type: object
                  required: ["name"]
                  properties:
                    name:
                      type: string
                    namespace:
                      type: string
                    userKey:
                      type: string
                      default: "username"
                    passwordKey:
                      type: string
                      default: "password"
                connectionTimeout:
                  type: integer
                  default: 30
                queryTimeout:
                  type: integer
                  default: 15
                sslMode:
                  type: string
                  enum: ["disable", "require", "verify-ca", "verify-full"]
                  default: "require"
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: ["Pending", "Connected", "Failed", "Degraded"]
                lastChecked:
                  type: string
                  format: date-time
                message:
                  type: string
                connectionPool:
                  type: object
                  properties:
                    active: 
                      type: integer
                    idle:
                      type: integer
      subresources:
        status: {}
      additionalPrinterColumns:
        - name: Type
          type: string
          jsonPath: .spec.type
        - name: Host
          type: string
          jsonPath: .spec.host
        - name: Status
          type: string
          jsonPath: .status.phase
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
```

### DBHubInstance CRD

```yaml
# crds/dbhub.tas.io_dbhubinstances.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: dbhubinstances.dbhub.tas.io
spec:
  group: dbhub.tas.io
  names:
    kind: DBHubInstance
    listKind: DBHubInstanceList
    plural: dbhubinstances
    singular: dbhubinstance
    shortNames:
      - dbhi
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          required: ["spec"]
          properties:
            spec:
              type: object
              properties:
                replicas:
                  type: integer
                  default: 1
                  minimum: 1
                image:
                  type: string
                  default: "bytebase/dbhub:latest"
                transport:
                  type: string
                  enum: ["http", "sse", "stdio"]
                  default: "http"
                port:
                  type: integer
                  default: 8080
                databaseSelector:
                  type: object
                  properties:
                    matchLabels:
                      type: object
                      additionalProperties:
                        type: string
                    matchNames:
                      type: array
                      items:
                        type: string
                defaultPolicy:
                  type: object
                  properties:
                    readonly:
                      type: boolean
                      default: true
                    maxRows:
                      type: integer
                      default: 500
                    allowedOperations:
                      type: array
                      items:
                        type: string
                        enum: ["execute_sql", "search_objects"]
                resources:
                  type: object
                  properties:
                    requests:
                      type: object
                      properties:
                        memory:
                          type: string
                        cpu:
                          type: string
                    limits:
                      type: object
                      properties:
                        memory:
                          type: string
                        cpu:
                          type: string
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: ["Pending", "Running", "Failed", "Degraded"]
                availableReplicas:
                  type: integer
                connectedDatabases:
                  type: array
                  items:
                    type: string
                endpoint:
                  type: string
                conditions:
                  type: array
                  items:
                    type: object
                    properties:
                      type:
                        type: string
                      status:
                        type: string
                      lastTransitionTime:
                        type: string
                        format: date-time
                      reason:
                        type: string
                      message:
                        type: string
      subresources:
        status: {}
        scale:
          specReplicasPath: .spec.replicas
          statusReplicasPath: .status.availableReplicas
      additionalPrinterColumns:
        - name: Replicas
          type: integer
          jsonPath: .spec.replicas
        - name: Available
          type: integer
          jsonPath: .status.availableReplicas
        - name: Databases
          type: string
          jsonPath: .status.connectedDatabases
        - name: Status
          type: string
          jsonPath: .status.phase
        - name: Endpoint
          type: string
          jsonPath: .status.endpoint
```

## Example Custom Resources

### Tenant Setup

```yaml
# examples/tenant-acme.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: tas-acme
  labels:
    tas.io/tenant: acme

---
apiVersion: v1
kind: Secret
metadata:
  name: analytics-db-creds
  namespace: tas-acme
type: Opaque
stringData:
  username: readonly_user
  password: super-secret-password

---
apiVersion: v1
kind: Secret
metadata:
  name: users-db-creds
  namespace: tas-acme
type: Opaque
stringData:
  username: readonly_user
  password: another-secret-password

---
apiVersion: dbhub.tas.io/v1alpha1
kind: Database
metadata:
  name: analytics-prod
  namespace: tas-acme
  labels:
    environment: production
    team: data
spec:
  type: postgres
  host: analytics-pg.acme.internal
  port: 5432
  database: analytics
  credentialsRef:
    name: analytics-db-creds
  sslMode: verify-full
  connectionTimeout: 30
  queryTimeout: 15

---
apiVersion: dbhub.tas.io/v1alpha1
kind: Database
metadata:
  name: users-prod
  namespace: tas-acme
  labels:
    environment: production
    team: platform
spec:
  type: mysql
  host: users-mysql.acme.internal
  port: 3306
  database: users
  credentialsRef:
    name: users-db-creds
  sslMode: require

---
apiVersion: dbhub.tas.io/v1alpha1
kind: DBHubInstance
metadata:
  name: acme-dbhub
  namespace: tas-acme
spec:
  replicas: 2
  transport: http
  port: 8080
  databaseSelector:
    matchLabels:
      environment: production
  defaultPolicy:
    readonly: true
    maxRows: 500
    allowedOperations:
      - execute_sql
      - search_objects
  resources:
    requests:
      memory: "128Mi"
      cpu: "100m"
    limits:
      memory: "512Mi"
      cpu: "500m"
```

## Controller Implementation

### Database Controller

```go
// internal/controller/database_controller.go
package controller

import (
	"context"
	"fmt"
	"time"

	dbhubv1alpha1 "github.com/tas-io/dbhub-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type DatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dbhub.tas.io,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbhub.tas.io,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var database dbhubv1alpha1.Database
	if err := r.Get(ctx, req.NamespacedName, &database); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch credentials
	secret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: database.Spec.CredentialsRef.Namespace,
		Name:      database.Spec.CredentialsRef.Name,
	}
	if secretName.Namespace == "" {
		secretName.Namespace = database.Namespace
	}

	if err := r.Get(ctx, secretName, secret); err != nil {
		logger.Error(err, "Failed to fetch credentials secret")
		database.Status.Phase = "Failed"
		database.Status.Message = fmt.Sprintf("Secret %s not found", secretName.Name)
		r.Status().Update(ctx, &database)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Test connection
	dsn := r.buildDSN(&database, secret)
	if err := r.testConnection(ctx, database.Spec.Type, dsn); err != nil {
		logger.Error(err, "Database connection failed")
		database.Status.Phase = "Failed"
		database.Status.Message = err.Error()
		r.Status().Update(ctx, &database)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// Update status
	database.Status.Phase = "Connected"
	database.Status.LastChecked = time.Now().Format(time.RFC3339)
	database.Status.Message = "Connection verified"

	if err := r.Status().Update(ctx, &database); err != nil {
		return ctrl.Result{}, err
	}

	// Recheck every 5 minutes
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *DatabaseReconciler) buildDSN(db *dbhubv1alpha1.Database, secret *corev1.Secret) string {
	userKey := db.Spec.CredentialsRef.UserKey
	passKey := db.Spec.CredentialsRef.PasswordKey
	if userKey == "" {
		userKey = "username"
	}
	if passKey == "" {
		passKey = "password"
	}

	user := string(secret.Data[userKey])
	pass := string(secret.Data[passKey])

	switch db.Spec.Type {
	case "postgres":
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			user, pass, db.Spec.Host, db.Spec.Port, db.Spec.Database, db.Spec.SSLMode)
	case "mysql", "mariadb":
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=%s",
			user, pass, db.Spec.Host, db.Spec.Port, db.Spec.Database, db.Spec.SSLMode)
	default:
		return ""
	}
}

func (r *DatabaseReconciler) testConnection(ctx context.Context, dbType, dsn string) error {
	// Implementation: attempt actual DB connection
	// Could use sql.Open() with appropriate driver
	return nil
}

func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbhubv1alpha1.Database{}).
		Complete(r)
}
```

### DBHubInstance Controller

```go
// internal/controller/dbhubinstance_controller.go
package controller

import (
	"context"
	"fmt"
	"strings"

	dbhubv1alpha1 "github.com/tas-io/dbhub-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type DBHubInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dbhub.tas.io,resources=dbhubinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbhub.tas.io,resources=dbhubinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbhub.tas.io,resources=databases,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete

func (r *DBHubInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var instance dbhubv1alpha1.DBHubInstance
	if err := r.Get(ctx, req.NamespacedName, &instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 1. Find matching databases
	databases, err := r.findMatchingDatabases(ctx, &instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found matching databases", "count", len(databases))

	// 2. Generate TOML config
	configMap, err := r.generateConfigMap(ctx, &instance, databases)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdate(ctx, configMap); err != nil {
		return ctrl.Result{}, err
	}

	// 3. Generate credentials secret (aggregated DSNs)
	secret, err := r.generateCredentialsSecret(ctx, &instance, databases)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdate(ctx, secret); err != nil {
		return ctrl.Result{}, err
	}

	// 4. Create/update deployment
	deployment := r.generateDeployment(&instance)
	if err := r.createOrUpdate(ctx, deployment); err != nil {
		return ctrl.Result{}, err
	}

	// 5. Create/update service
	service := r.generateService(&instance)
	if err := r.createOrUpdate(ctx, service); err != nil {
		return ctrl.Result{}, err
	}

	// 6. Update status
	instance.Status.Phase = "Running"
	instance.Status.Endpoint = fmt.Sprintf("%s.%s.svc:%d",
		instance.Name, instance.Namespace, instance.Spec.Port)
	instance.Status.ConnectedDatabases = make([]string, len(databases))
	for i, db := range databases {
		instance.Status.ConnectedDatabases[i] = db.Name
	}

	if err := r.Status().Update(ctx, &instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DBHubInstanceReconciler) findMatchingDatabases(
	ctx context.Context,
	instance *dbhubv1alpha1.DBHubInstance,
) ([]dbhubv1alpha1.Database, error) {
	var allDatabases dbhubv1alpha1.DatabaseList
	if err := r.List(ctx, &allDatabases, client.InNamespace(instance.Namespace)); err != nil {
		return nil, err
	}

	var matched []dbhubv1alpha1.Database
	for _, db := range allDatabases.Items {
		if r.matchesSelector(&db, instance.Spec.DatabaseSelector) {
			matched = append(matched, db)
		}
	}
	return matched, nil
}

func (r *DBHubInstanceReconciler) matchesSelector(
	db *dbhubv1alpha1.Database,
	selector *dbhubv1alpha1.DatabaseSelector,
) bool {
	if selector == nil {
		return true
	}

	// Check label selector
	if selector.MatchLabels != nil {
		for k, v := range selector.MatchLabels {
			if db.Labels[k] != v {
				return false
			}
		}
	}

	// Check name selector
	if len(selector.MatchNames) > 0 {
		found := false
		for _, name := range selector.MatchNames {
			if db.Name == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (r *DBHubInstanceReconciler) generateConfigMap(
	ctx context.Context,
	instance *dbhubv1alpha1.DBHubInstance,
	databases []dbhubv1alpha1.Database,
) (*corev1.ConfigMap, error) {
	toml := ""
	for _, db := range databases {
		toml += fmt.Sprintf(`
[[sources]]
id = "%s"
dsn = "${%s_DSN}"
connection_timeout = %d
query_timeout = %d
`, db.Name, sanitizeEnvName(db.Name), db.Spec.ConnectionTimeout, db.Spec.QueryTimeout)
	}

	// Add default tools
	policy := instance.Spec.DefaultPolicy
	for _, op := range policy.AllowedOperations {
		toml += fmt.Sprintf(`
[[tools]]
name = "%s"
source = "*"
readonly = %t
max_rows = %d
`, op, policy.Readonly, policy.MaxRows)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-config",
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, dbhubv1alpha1.GroupVersion.WithKind("DBHubInstance")),
			},
		},
		Data: map[string]string{
			"dbhub.toml": toml,
		},
	}, nil
}

func (r *DBHubInstanceReconciler) generateCredentialsSecret(
	ctx context.Context,
	instance *dbhubv1alpha1.DBHubInstance,
	databases []dbhubv1alpha1.Database,
) (*corev1.Secret, error) {
	data := make(map[string][]byte)

	for _, db := range databases {
		// Fetch the database's credentials secret
		secret := &corev1.Secret{}
		secretName := client.ObjectKey{
			Namespace: db.Namespace,
			Name:      db.Spec.CredentialsRef.Name,
		}
		if db.Spec.CredentialsRef.Namespace != "" {
			secretName.Namespace = db.Spec.CredentialsRef.Namespace
		}

		if err := r.Get(ctx, secretName, secret); err != nil {
			return nil, err
		}

		userKey := db.Spec.CredentialsRef.UserKey
		passKey := db.Spec.CredentialsRef.PasswordKey
		if userKey == "" {
			userKey = "username"
		}
		if passKey == "" {
			passKey = "password"
		}

		user := string(secret.Data[userKey])
		pass := string(secret.Data[passKey])

		var dsn string
		switch db.Spec.Type {
		case "postgres":
			dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
				user, pass, db.Spec.Host, db.Spec.Port, db.Spec.Database, db.Spec.SSLMode)
		case "mysql", "mariadb":
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=%s",
				user, pass, db.Spec.Host, db.Spec.Port, db.Spec.Database, db.Spec.SSLMode)
		}

		envKey := sanitizeEnvName(db.Name) + "_DSN"
		data[envKey] = []byte(dsn)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-creds",
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, dbhubv1alpha1.GroupVersion.WithKind("DBHubInstance")),
			},
		},
		Data: data,
	}, nil
}

func (r *DBHubInstanceReconciler) generateDeployment(instance *dbhubv1alpha1.DBHubInstance) *appsv1.Deployment {
	replicas := int32(instance.Spec.Replicas)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, dbhubv1alpha1.GroupVersion.WithKind("DBHubInstance")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": instance.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": instance.Name},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:    "config-renderer",
						Image:   "bhgedigital/envsubst:latest",
						Command: []string{"sh", "-c", "envsubst < /config-template/dbhub.toml > /config/dbhub.toml"},
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: instance.Name + "-creds",
								},
							},
						}},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "config-template", MountPath: "/config-template"},
							{Name: "config-rendered", MountPath: "/config"},
						},
					}},
					Containers: []corev1.Container{{
						Name:  "dbhub",
						Image: instance.Spec.Image,
						Args: []string{
							"--config", "/etc/dbhub/dbhub.toml",
							"--transport", instance.Spec.Transport,
							"--port", fmt.Sprintf("%d", instance.Spec.Port),
						},
						Ports: []corev1.ContainerPort{{
							ContainerPort: int32(instance.Spec.Port),
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "config-rendered",
							MountPath: "/etc/dbhub",
						}},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.FromInt(instance.Spec.Port),
								},
							},
							InitialDelaySeconds: 10,
							PeriodSeconds:       30,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.FromInt(instance.Spec.Port),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "config-template",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Name + "-config",
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
}

func (r *DBHubInstanceReconciler) generateService(instance *dbhubv1alpha1.DBHubInstance) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, dbhubv1alpha1.GroupVersion.WithKind("DBHubInstance")),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": instance.Name},
			Ports: []corev1.ServicePort{{
				Port:       int32(instance.Spec.Port),
				TargetPort: intstr.FromInt(instance.Spec.Port),
				Name:       "http",
			}},
		},
	}
}

func (r *DBHubInstanceReconciler) createOrUpdate(ctx context.Context, obj client.Object) error {
	// Implementation: server-side apply or create/update pattern
	return nil
}

func (r *DBHubInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbhubv1alpha1.DBHubInstance{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

func sanitizeEnvName(name string) string {
	// Convert to uppercase, replace hyphens with underscores
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}
```

## API Types

```go
// api/v1alpha1/database_types.go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DatabaseSpec struct {
	Type              string            `json:"type"`
	Host              string            `json:"host,omitempty"`
	Port              int               `json:"port,omitempty"`
	Database          string            `json:"database,omitempty"`
	CredentialsRef    CredentialsRef    `json:"credentialsRef"`
	ConnectionTimeout int               `json:"connectionTimeout,omitempty"`
	QueryTimeout      int               `json:"queryTimeout,omitempty"`
	SSLMode           string            `json:"sslMode,omitempty"`
}

type CredentialsRef struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace,omitempty"`
	UserKey     string `json:"userKey,omitempty"`
	PasswordKey string `json:"passwordKey,omitempty"`
}

type DatabaseStatus struct {
	Phase          string          `json:"phase,omitempty"`
	LastChecked    string          `json:"lastChecked,omitempty"`
	Message        string          `json:"message,omitempty"`
	ConnectionPool *ConnectionPool `json:"connectionPool,omitempty"`
}

type ConnectionPool struct {
	Active int `json:"active,omitempty"`
	Idle   int `json:"idle,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec   `json:"spec,omitempty"`
	Status DatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}
```

```go
// api/v1alpha1/dbhubinstance_types.go
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DBHubInstanceSpec struct {
	Replicas         int               `json:"replicas,omitempty"`
	Image            string            `json:"image,omitempty"`
	Transport        string            `json:"transport,omitempty"`
	Port             int               `json:"port,omitempty"`
	DatabaseSelector *DatabaseSelector `json:"databaseSelector,omitempty"`
	DefaultPolicy    *DefaultPolicy    `json:"defaultPolicy,omitempty"`
	Resources        *Resources        `json:"resources,omitempty"`
}

type DatabaseSelector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	MatchNames  []string          `json:"matchNames,omitempty"`
}

type DefaultPolicy struct {
	Readonly          bool     `json:"readonly,omitempty"`
	MaxRows           int      `json:"maxRows,omitempty"`
	AllowedOperations []string `json:"allowedOperations,omitempty"`
}

type Resources struct {
	Requests corev1.ResourceList `json:"requests,omitempty"`
	Limits   corev1.ResourceList `json:"limits,omitempty"`
}

type DBHubInstanceStatus struct {
	Phase              string      `json:"phase,omitempty"`
	AvailableReplicas  int         `json:"availableReplicas,omitempty"`
	ConnectedDatabases []string    `json:"connectedDatabases,omitempty"`
	Endpoint           string      `json:"endpoint,omitempty"`
	Conditions         []Condition `json:"conditions,omitempty"`
}

type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.availableReplicas
type DBHubInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DBHubInstanceSpec   `json:"spec,omitempty"`
	Status DBHubInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DBHubInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBHubInstance `json:"items"`
}
```

## External Secrets Integration

```yaml
# external-secrets/clustersecretstore.yaml
apiVersion: external-secrets.io/v1beta1
kind: ClusterSecretStore
metadata:
  name: vault-backend
spec:
  provider:
    vault:
      server: "https://vault.example.com"
      path: "secret"
      version: "v2"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "dbhub-operator"

---
# external-secrets/database-externalsecret.yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: analytics-db-creds
  namespace: tas-acme
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: analytics-db-creds
  data:
    - secretKey: username
      remoteRef:
        key: tas/acme/analytics-db
        property: username
    - secretKey: password
      remoteRef:
        key: tas/acme/analytics-db
        property: password
```

## Operator Scaffolding

```bash
# Initialize with kubebuilder
kubebuilder init --domain tas.io --repo github.com/tas-io/dbhub-operator
kubebuilder create api --group dbhub --version v1alpha1 --kind Database
kubebuilder create api --group dbhub --version v1alpha1 --kind DBHubInstance

# Generate manifests
make manifests

# Build and push
make docker-build docker-push IMG=your-registry/dbhub-operator:v0.1.0

# Deploy to cluster
make deploy IMG=your-registry/dbhub-operator:v0.1.0
```

## Usage

```bash
# Apply CRDs
kubectl apply -f crds/

# Deploy operator
kubectl apply -f deploy/

# Create a tenant namespace with databases
kubectl apply -f examples/tenant-acme.yaml

# Check status
kubectl get databases -n tas-acme
# NAME             TYPE       HOST                        STATUS      AGE
# analytics-prod   postgres   analytics-pg.acme.internal  Connected   5m
# users-prod       mysql      users-mysql.acme.internal   Connected   5m

kubectl get dbhubinstances -n tas-acme
# NAME          REPLICAS   AVAILABLE   DATABASES                    STATUS    ENDPOINT
# acme-dbhub    2          2           [analytics-prod,users-prod]  Running   acme-dbhub.tas-acme.svc:8080

# Scale
kubectl scale dbhubinstance acme-dbhub -n tas-acme --replicas=3

# View generated config
kubectl get configmap acme-dbhub-config -n tas-acme -o yaml
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────┐     ┌──────────────────────────────────┐  │
│  │  DBHub Operator  │     │         Tenant Namespace          │  │
│  │                  │     │          (tas-acme)               │  │
│  │  ┌────────────┐  │     │                                   │  │
│  │  │ Database   │──┼────▶│  ┌─────────┐    ┌─────────┐      │  │
│  │  │ Controller │  │     │  │Database │    │Database │      │  │
│  │  └────────────┘  │     │  │analytics│    │users    │      │  │
│  │                  │     │  └────┬────┘    └────┬────┘      │  │
│  │  ┌────────────┐  │     │       │              │           │  │
│  │  │ DBHub      │──┼────▶│       ▼              ▼           │  │
│  │  │ Instance   │  │     │  ┌─────────────────────────┐     │  │
│  │  │ Controller │  │     │  │    DBHubInstance        │     │  │
│  │  └────────────┘  │     │  │    (acme-dbhub)         │     │  │
│  │                  │     │  │                         │     │  │
│  └──────────────────┘     │  │  ┌───────┐ ┌───────┐   │     │  │
│                           │  │  │Pod    │ │Pod    │   │     │  │
│                           │  │  │dbhub  │ │dbhub  │   │     │  │
│                           │  │  └───────┘ └───────┘   │     │  │
│                           │  └─────────────────────────┘     │  │
│                           │                                   │  │
│                           └──────────────────────────────────┘  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```
