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
	"database/sql"
	"fmt"
	"net/url"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dbhubv1alpha1 "github.com/Tributary-ai-services/dbhub-operator/api/v1alpha1"

	// Database drivers
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/microsoft/go-mssqldb"
	_ "modernc.org/sqlite"
)

const (
	// HealthCheckInterval is how often to recheck database connections
	HealthCheckInterval = 5 * time.Minute

	// ConnectionTestTimeout is the timeout for connection tests
	ConnectionTestTimeout = 10 * time.Second

	// Condition types
	ConditionTypeReady     = "Ready"
	ConditionTypeConnected = "Connected"
)

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dbhub.tas.io,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbhub.tas.io,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbhub.tas.io,resources=databases/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Database", "name", req.Name, "namespace", req.Namespace)

	// Fetch the Database instance
	database := &dbhubv1alpha1.Database{}
	if err := r.Get(ctx, req.NamespacedName, database); err != nil {
		if apierrors.IsNotFound(err) {
			// Database was deleted
			logger.Info("Database resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Database")
		return ctrl.Result{}, err
	}

	// Update observed generation
	if database.Status.ObservedGeneration != database.Generation {
		database.Status.ObservedGeneration = database.Generation
	}

	// Set initial phase if not set
	if database.Status.Phase == "" {
		database.Status.Phase = dbhubv1alpha1.DatabasePhasePending
	}

	// Fetch credentials from Secret
	username, password, err := r.getCredentials(ctx, database)
	if err != nil {
		logger.Error(err, "Failed to get credentials")
		r.setDatabaseStatus(ctx, database, dbhubv1alpha1.DatabasePhaseFailed,
			fmt.Sprintf("Failed to get credentials: %v", err))
		return ctrl.Result{RequeueAfter: HealthCheckInterval}, nil
	}

	// Build DSN
	dsn, dsnWithoutCreds := r.buildDSN(database, username, password)
	database.Status.DSN = dsnWithoutCreds

	// Test database connection
	err = r.testConnection(ctx, database, dsn)
	now := metav1.Now()
	database.Status.LastChecked = &now

	if err != nil {
		logger.Error(err, "Database connection test failed")
		r.setDatabaseStatus(ctx, database, dbhubv1alpha1.DatabasePhaseFailed,
			fmt.Sprintf("Connection failed: %v", err))

		// Set condition
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeConnected,
			Status:             metav1.ConditionFalse,
			Reason:             "ConnectionFailed",
			Message:            err.Error(),
			LastTransitionTime: now,
		})
	} else {
		logger.Info("Database connection successful")
		r.setDatabaseStatus(ctx, database, dbhubv1alpha1.DatabasePhaseConnected, "")

		// Set condition
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeConnected,
			Status:             metav1.ConditionTrue,
			Reason:             "ConnectionSuccessful",
			Message:            "Successfully connected to database",
			LastTransitionTime: now,
		})
	}

	// Update status
	if err := r.Status().Update(ctx, database); err != nil {
		logger.Error(err, "Failed to update Database status")
		return ctrl.Result{}, err
	}

	// Requeue for periodic health check
	logger.Info("Scheduling next health check", "interval", HealthCheckInterval)
	return ctrl.Result{RequeueAfter: HealthCheckInterval}, nil
}

// getCredentials fetches username and password from the referenced Secret
func (r *DatabaseReconciler) getCredentials(ctx context.Context, database *dbhubv1alpha1.Database) (string, string, error) {
	secretNamespace := database.GetCredentialsNamespace()
	secretName := database.Spec.CredentialsRef.Name

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: secretNamespace,
		Name:      secretName,
	}, secret); err != nil {
		return "", "", fmt.Errorf("failed to get secret %s/%s: %w", secretNamespace, secretName, err)
	}

	userKey := database.GetUserKey()
	passwordKey := database.GetPasswordKey()

	username, ok := secret.Data[userKey]
	if !ok {
		return "", "", fmt.Errorf("secret %s/%s does not contain key %s", secretNamespace, secretName, userKey)
	}

	password, ok := secret.Data[passwordKey]
	if !ok {
		return "", "", fmt.Errorf("secret %s/%s does not contain key %s", secretNamespace, secretName, passwordKey)
	}

	return string(username), string(password), nil
}

// buildDSN constructs the data source name for the database
// Returns full DSN (with credentials) and DSN without credentials (for status)
func (r *DatabaseReconciler) buildDSN(database *dbhubv1alpha1.Database, username, password string) (string, string) {
	spec := database.Spec

	switch spec.Type {
	case dbhubv1alpha1.DatabaseTypePostgres:
		// postgres://user:password@host:port/database?sslmode=disable
		dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			url.QueryEscape(username),
			url.QueryEscape(password),
			spec.Host,
			spec.Port,
			spec.Database,
			spec.SSLMode,
		)
		dsnWithoutCreds := fmt.Sprintf("postgres://%s:%d/%s?sslmode=%s",
			spec.Host, spec.Port, spec.Database, spec.SSLMode)
		return dsn, dsnWithoutCreds

	case dbhubv1alpha1.DatabaseTypeMySQL, dbhubv1alpha1.DatabaseTypeMariaDB:
		// user:password@tcp(host:port)/database?tls=false
		tls := "false"
		if spec.SSLMode == dbhubv1alpha1.SSLModeRequire || spec.SSLMode == dbhubv1alpha1.SSLModeVerifyCA || spec.SSLMode == dbhubv1alpha1.SSLModeVerifyFull {
			tls = "true"
		}
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=%s&timeout=%ds",
			username,
			password,
			spec.Host,
			spec.Port,
			spec.Database,
			tls,
			spec.ConnectionTimeout,
		)
		dsnWithoutCreds := fmt.Sprintf("tcp(%s:%d)/%s?tls=%s",
			spec.Host, spec.Port, spec.Database, tls)
		return dsn, dsnWithoutCreds

	case dbhubv1alpha1.DatabaseTypeSQLServer:
		// sqlserver://user:password@host:port?database=dbname
		dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s&connection+timeout=%d",
			url.QueryEscape(username),
			url.QueryEscape(password),
			spec.Host,
			spec.Port,
			spec.Database,
			spec.ConnectionTimeout,
		)
		dsnWithoutCreds := fmt.Sprintf("sqlserver://%s:%d?database=%s",
			spec.Host, spec.Port, spec.Database)
		return dsn, dsnWithoutCreds

	case dbhubv1alpha1.DatabaseTypeSQLite:
		// For SQLite, the database field is the file path
		// Credentials are typically not used for SQLite
		dsn := spec.Database
		return dsn, dsn

	default:
		// Unknown database type, return empty
		return "", ""
	}
}

// testConnection tests the database connection
func (r *DatabaseReconciler) testConnection(ctx context.Context, database *dbhubv1alpha1.Database, dsn string) error {
	var driverName string
	switch database.Spec.Type {
	case dbhubv1alpha1.DatabaseTypePostgres:
		driverName = "postgres"
	case dbhubv1alpha1.DatabaseTypeMySQL, dbhubv1alpha1.DatabaseTypeMariaDB:
		driverName = "mysql"
	case dbhubv1alpha1.DatabaseTypeSQLServer:
		driverName = "sqlserver"
	case dbhubv1alpha1.DatabaseTypeSQLite:
		driverName = "sqlite"
	default:
		return fmt.Errorf("unsupported database type: %s", database.Spec.Type)
	}

	// Create connection with timeout
	ctx, cancel := context.WithTimeout(ctx, ConnectionTestTimeout)
	defer cancel()

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	// Set connection pool settings
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(ConnectionTestTimeout)

	// Ping the database
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	return nil
}

// setDatabaseStatus updates the database status
func (r *DatabaseReconciler) setDatabaseStatus(ctx context.Context, database *dbhubv1alpha1.Database, phase dbhubv1alpha1.DatabasePhase, message string) {
	database.Status.Phase = phase
	database.Status.Message = message

	// Set Ready condition based on phase
	now := metav1.Now()
	if phase == dbhubv1alpha1.DatabasePhaseConnected {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			Reason:             "DatabaseReady",
			Message:            "Database is connected and ready",
			LastTransitionTime: now,
		})
	} else {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             string(phase),
			Message:            message,
			LastTransitionTime: now,
		})
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbhubv1alpha1.Database{}).
		Named("database").
		Complete(r)
}
