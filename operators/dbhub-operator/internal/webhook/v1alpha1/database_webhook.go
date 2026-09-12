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

package v1alpha1

import (
	"context"
	"fmt"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	dbhubv1alpha1 "github.com/Tributary-ai-services/dbhub-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var databaselog = logf.Log.WithName("database-resource")

// SetupDatabaseWebhookWithManager registers the webhook for Database in the manager.
func SetupDatabaseWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &dbhubv1alpha1.Database{}).
		WithValidator(&DatabaseCustomValidator{}).
		WithDefaulter(&DatabaseCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-dbhub-tas-io-v1alpha1-database,mutating=true,failurePolicy=fail,sideEffects=None,groups=dbhub.tas.io,resources=databases,verbs=create;update,versions=v1alpha1,name=mdatabase-v1alpha1.kb.io,admissionReviewVersions=v1

// DatabaseCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Database when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type DatabaseCustomDefaulter struct{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Database.
func (d *DatabaseCustomDefaulter) Default(_ context.Context, obj *dbhubv1alpha1.Database) error {
	databaselog.Info("Defaulting for Database", "name", obj.GetName())

	// Default port based on database type
	if obj.Spec.Port == 0 {
		switch obj.Spec.Type {
		case dbhubv1alpha1.DatabaseTypePostgres:
			obj.Spec.Port = 5432
		case dbhubv1alpha1.DatabaseTypeMySQL, dbhubv1alpha1.DatabaseTypeMariaDB:
			obj.Spec.Port = 3306
		case dbhubv1alpha1.DatabaseTypeSQLServer:
			obj.Spec.Port = 1433
		}
	}

	// Default SSL mode
	if obj.Spec.SSLMode == "" {
		obj.Spec.SSLMode = dbhubv1alpha1.SSLModeDisable
	}

	// Default connection timeout
	if obj.Spec.ConnectionTimeout == 0 {
		obj.Spec.ConnectionTimeout = 30
	}

	// Default query timeout
	if obj.Spec.QueryTimeout == 0 {
		obj.Spec.QueryTimeout = 60
	}

	// Default max rows
	if obj.Spec.MaxRows == 0 {
		obj.Spec.MaxRows = 1000
	}

	// Default credentials keys
	if obj.Spec.CredentialsRef.UserKey == "" {
		obj.Spec.CredentialsRef.UserKey = "username"
	}
	if obj.Spec.CredentialsRef.PasswordKey == "" {
		obj.Spec.CredentialsRef.PasswordKey = "password"
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-dbhub-tas-io-v1alpha1-database,mutating=false,failurePolicy=fail,sideEffects=None,groups=dbhub.tas.io,resources=databases,verbs=create;update,versions=v1alpha1,name=vdatabase-v1alpha1.kb.io,admissionReviewVersions=v1

// DatabaseCustomValidator struct is responsible for validating the Database resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type DatabaseCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Database.
func (v *DatabaseCustomValidator) ValidateCreate(_ context.Context, obj *dbhubv1alpha1.Database) (admission.Warnings, error) {
	databaselog.Info("Validation for Database upon creation", "name", obj.GetName())
	return v.validateDatabase(obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Database.
func (v *DatabaseCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *dbhubv1alpha1.Database) (admission.Warnings, error) {
	databaselog.Info("Validation for Database upon update", "name", newObj.GetName())

	var warnings admission.Warnings

	// Warn if database type is being changed
	if oldObj.Spec.Type != newObj.Spec.Type {
		warnings = append(warnings, "Changing database type may require credential updates")
	}

	// Warn if host is being changed
	if oldObj.Spec.Host != newObj.Spec.Host {
		warnings = append(warnings, "Changing host will trigger reconnection")
	}

	validationWarnings, err := v.validateDatabase(newObj)
	warnings = append(warnings, validationWarnings...)

	return warnings, err
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Database.
func (v *DatabaseCustomValidator) ValidateDelete(_ context.Context, obj *dbhubv1alpha1.Database) (admission.Warnings, error) {
	databaselog.Info("Validation for Database upon deletion", "name", obj.GetName())
	return nil, nil
}

// validateDatabase performs validation on the Database spec
func (v *DatabaseCustomValidator) validateDatabase(db *dbhubv1alpha1.Database) (admission.Warnings, error) {
	var allErrs field.ErrorList
	var warnings admission.Warnings

	specPath := field.NewPath("spec")

	// Validate database type
	validTypes := map[dbhubv1alpha1.DatabaseType]bool{
		dbhubv1alpha1.DatabaseTypePostgres:  true,
		dbhubv1alpha1.DatabaseTypeMySQL:     true,
		dbhubv1alpha1.DatabaseTypeMariaDB:   true,
		dbhubv1alpha1.DatabaseTypeSQLServer: true,
		dbhubv1alpha1.DatabaseTypeSQLite:    true,
	}
	if !validTypes[db.Spec.Type] {
		allErrs = append(allErrs, field.NotSupported(
			specPath.Child("type"),
			db.Spec.Type,
			[]string{
				string(dbhubv1alpha1.DatabaseTypePostgres),
				string(dbhubv1alpha1.DatabaseTypeMySQL),
				string(dbhubv1alpha1.DatabaseTypeMariaDB),
				string(dbhubv1alpha1.DatabaseTypeSQLServer),
				string(dbhubv1alpha1.DatabaseTypeSQLite),
			},
		))
	}

	// Validate host (not required for SQLite)
	if db.Spec.Type != dbhubv1alpha1.DatabaseTypeSQLite {
		if db.Spec.Host == "" {
			allErrs = append(allErrs, field.Required(
				specPath.Child("host"),
				"host is required for non-SQLite databases",
			))
		} else {
			// Validate host is a valid hostname or IP
			if ip := net.ParseIP(db.Spec.Host); ip == nil {
				// Not an IP, check if it's a valid hostname
				if len(db.Spec.Host) > 253 {
					allErrs = append(allErrs, field.Invalid(
						specPath.Child("host"),
						db.Spec.Host,
						"hostname exceeds maximum length of 253 characters",
					))
				}
			}
		}
	}

	// Validate port
	if db.Spec.Port < 0 || db.Spec.Port > 65535 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("port"),
			db.Spec.Port,
			"port must be between 0 and 65535",
		))
	}

	// Validate database name
	if db.Spec.Database == "" {
		allErrs = append(allErrs, field.Required(
			specPath.Child("database"),
			"database name is required",
		))
	}

	// Validate credentials reference (not required for SQLite)
	if db.Spec.Type != dbhubv1alpha1.DatabaseTypeSQLite {
		if db.Spec.CredentialsRef.Name == "" {
			allErrs = append(allErrs, field.Required(
				specPath.Child("credentialsRef", "name"),
				"credentials secret name is required for non-SQLite databases",
			))
		}
	}

	// Validate SSL mode
	validSSLModes := map[dbhubv1alpha1.SSLMode]bool{
		dbhubv1alpha1.SSLModeDisable:    true,
		dbhubv1alpha1.SSLModeRequire:    true,
		dbhubv1alpha1.SSLModeVerifyCA:   true,
		dbhubv1alpha1.SSLModeVerifyFull: true,
	}
	if db.Spec.SSLMode != "" && !validSSLModes[db.Spec.SSLMode] {
		allErrs = append(allErrs, field.NotSupported(
			specPath.Child("sslMode"),
			db.Spec.SSLMode,
			[]string{
				string(dbhubv1alpha1.SSLModeDisable),
				string(dbhubv1alpha1.SSLModeRequire),
				string(dbhubv1alpha1.SSLModeVerifyCA),
				string(dbhubv1alpha1.SSLModeVerifyFull),
			},
		))
	}

	// Validate timeouts
	if db.Spec.ConnectionTimeout < 0 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("connectionTimeout"),
			db.Spec.ConnectionTimeout,
			"connection timeout must be non-negative",
		))
	}
	if db.Spec.QueryTimeout < 0 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("queryTimeout"),
			db.Spec.QueryTimeout,
			"query timeout must be non-negative",
		))
	}

	// Validate max rows
	if db.Spec.MaxRows < 0 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("maxRows"),
			db.Spec.MaxRows,
			"max rows must be non-negative",
		))
	}
	if db.Spec.MaxRows > 100000 {
		warnings = append(warnings, fmt.Sprintf("maxRows=%d is very high and may impact performance", db.Spec.MaxRows))
	}

	// Warn about insecure SSL mode
	if db.Spec.SSLMode == dbhubv1alpha1.SSLModeDisable {
		warnings = append(warnings, "SSL is disabled, connection is not encrypted")
	}

	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "dbhub.tas.io", Kind: "Database"},
			db.Name,
			allErrs,
		)
	}

	return warnings, nil
}
