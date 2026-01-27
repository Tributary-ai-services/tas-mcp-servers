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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

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
var dbhubinstancelog = logf.Log.WithName("dbhubinstance-resource")

// SetupDBHubInstanceWebhookWithManager registers the webhook for DBHubInstance in the manager.
func SetupDBHubInstanceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &dbhubv1alpha1.DBHubInstance{}).
		WithValidator(&DBHubInstanceCustomValidator{}).
		WithDefaulter(&DBHubInstanceCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-dbhub-tas-io-v1alpha1-dbhubinstance,mutating=true,failurePolicy=fail,sideEffects=None,groups=dbhub.tas.io,resources=dbhubinstances,verbs=create;update,versions=v1alpha1,name=mdbhubinstance-v1alpha1.kb.io,admissionReviewVersions=v1

// DBHubInstanceCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind DBHubInstance when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type DBHubInstanceCustomDefaulter struct{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind DBHubInstance.
func (d *DBHubInstanceCustomDefaulter) Default(_ context.Context, obj *dbhubv1alpha1.DBHubInstance) error {
	dbhubinstancelog.Info("Defaulting for DBHubInstance", "name", obj.GetName())

	// Default replicas
	if obj.Spec.Replicas == nil {
		replicas := int32(1)
		obj.Spec.Replicas = &replicas
	}

	// Default image
	if obj.Spec.Image == "" {
		obj.Spec.Image = "bytebase/dbhub:latest"
	}

	// Default image pull policy
	if obj.Spec.ImagePullPolicy == "" {
		obj.Spec.ImagePullPolicy = corev1.PullIfNotPresent
	}

	// Default transport
	if obj.Spec.Transport == "" {
		obj.Spec.Transport = dbhubv1alpha1.TransportTypeHTTP
	}

	// Default port
	if obj.Spec.Port == 0 {
		obj.Spec.Port = 8080
	}

	// Default resources
	if obj.Spec.Resources == nil {
		obj.Spec.Resources = &dbhubv1alpha1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		}
	}

	// Default policy
	if obj.Spec.DefaultPolicy == nil {
		obj.Spec.DefaultPolicy = &dbhubv1alpha1.DefaultPolicy{
			ReadOnly: true,
			MaxRows:  1000,
			AllowedOperations: []string{
				"execute_sql",
				"search_objects",
			},
		}
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-dbhub-tas-io-v1alpha1-dbhubinstance,mutating=false,failurePolicy=fail,sideEffects=None,groups=dbhub.tas.io,resources=dbhubinstances,verbs=create;update,versions=v1alpha1,name=vdbhubinstance-v1alpha1.kb.io,admissionReviewVersions=v1

// DBHubInstanceCustomValidator struct is responsible for validating the DBHubInstance resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type DBHubInstanceCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type DBHubInstance.
func (v *DBHubInstanceCustomValidator) ValidateCreate(_ context.Context, obj *dbhubv1alpha1.DBHubInstance) (admission.Warnings, error) {
	dbhubinstancelog.Info("Validation for DBHubInstance upon creation", "name", obj.GetName())
	return v.validateDBHubInstance(obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type DBHubInstance.
func (v *DBHubInstanceCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *dbhubv1alpha1.DBHubInstance) (admission.Warnings, error) {
	dbhubinstancelog.Info("Validation for DBHubInstance upon update", "name", newObj.GetName())

	var warnings admission.Warnings

	// Warn if transport is changing
	if oldObj.Spec.Transport != newObj.Spec.Transport {
		warnings = append(warnings, "Changing transport type will restart all pods")
	}

	// Warn if port is changing
	if oldObj.Spec.Port != newObj.Spec.Port {
		warnings = append(warnings, "Changing port will restart all pods and may require service reconfiguration")
	}

	validationWarnings, err := v.validateDBHubInstance(newObj)
	warnings = append(warnings, validationWarnings...)

	return warnings, err
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type DBHubInstance.
func (v *DBHubInstanceCustomValidator) ValidateDelete(_ context.Context, obj *dbhubv1alpha1.DBHubInstance) (admission.Warnings, error) {
	dbhubinstancelog.Info("Validation for DBHubInstance upon deletion", "name", obj.GetName())
	return nil, nil
}

// validateDBHubInstance performs validation on the DBHubInstance spec
func (v *DBHubInstanceCustomValidator) validateDBHubInstance(instance *dbhubv1alpha1.DBHubInstance) (admission.Warnings, error) {
	var allErrs field.ErrorList
	var warnings admission.Warnings

	specPath := field.NewPath("spec")

	// Validate replicas
	if instance.Spec.Replicas != nil && *instance.Spec.Replicas < 0 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("replicas"),
			*instance.Spec.Replicas,
			"replicas must be non-negative",
		))
	}
	if instance.Spec.Replicas != nil && *instance.Spec.Replicas > 10 {
		warnings = append(warnings, fmt.Sprintf("replicas=%d is high for DBHub instances", *instance.Spec.Replicas))
	}

	// Validate transport
	validTransports := map[dbhubv1alpha1.TransportType]bool{
		dbhubv1alpha1.TransportTypeHTTP:  true,
		dbhubv1alpha1.TransportTypeSSE:   true,
		dbhubv1alpha1.TransportTypeStdio: true,
	}
	if instance.Spec.Transport != "" && !validTransports[instance.Spec.Transport] {
		allErrs = append(allErrs, field.NotSupported(
			specPath.Child("transport"),
			instance.Spec.Transport,
			[]string{
				string(dbhubv1alpha1.TransportTypeHTTP),
				string(dbhubv1alpha1.TransportTypeSSE),
				string(dbhubv1alpha1.TransportTypeStdio),
			},
		))
	}

	// Validate port
	if instance.Spec.Port < 0 || instance.Spec.Port > 65535 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("port"),
			instance.Spec.Port,
			"port must be between 0 and 65535",
		))
	}
	if instance.Spec.Port > 0 && instance.Spec.Port < 1024 {
		warnings = append(warnings, "Using a privileged port (<1024) may require special container security context")
	}

	// Validate database selector
	if instance.Spec.DatabaseSelector != nil {
		selectorPath := specPath.Child("databaseSelector")

		// Must have at least one selector criterion
		if len(instance.Spec.DatabaseSelector.MatchLabels) == 0 && len(instance.Spec.DatabaseSelector.MatchNames) == 0 {
			allErrs = append(allErrs, field.Required(
				selectorPath,
				"databaseSelector must have at least one of matchLabels or matchNames",
			))
		}
	}

	// Validate default policy
	if instance.Spec.DefaultPolicy != nil {
		policyPath := specPath.Child("defaultPolicy")

		if instance.Spec.DefaultPolicy.MaxRows < 0 {
			allErrs = append(allErrs, field.Invalid(
				policyPath.Child("maxRows"),
				instance.Spec.DefaultPolicy.MaxRows,
				"maxRows must be non-negative",
			))
		}

		if instance.Spec.DefaultPolicy.MaxRows > 100000 {
			warnings = append(warnings, fmt.Sprintf("defaultPolicy.maxRows=%d is very high", instance.Spec.DefaultPolicy.MaxRows))
		}

		// Warn about non-readonly mode
		if !instance.Spec.DefaultPolicy.ReadOnly {
			warnings = append(warnings, "defaultPolicy.readonly is false - write operations are enabled, use with caution")
		}

		// Validate allowed operations
		validOperations := map[string]bool{
			"execute_sql":     true,
			"search_objects":  true,
			"list_tables":     true,
			"describe_table":  true,
			"list_connectors": true,
		}
		for i, op := range instance.Spec.DefaultPolicy.AllowedOperations {
			if !validOperations[op] {
				warnings = append(warnings, fmt.Sprintf("defaultPolicy.allowedOperations[%d]=%s may not be a recognized operation", i, op))
			}
		}
	}

	// Validate resources
	if instance.Spec.Resources != nil {
		resourcesPath := specPath.Child("resources")

		// Check that limits >= requests
		if instance.Spec.Resources.Requests != nil && instance.Spec.Resources.Limits != nil {
			requestCPU := instance.Spec.Resources.Requests[corev1.ResourceCPU]
			limitCPU := instance.Spec.Resources.Limits[corev1.ResourceCPU]
			if !limitCPU.IsZero() && requestCPU.Cmp(limitCPU) > 0 {
				allErrs = append(allErrs, field.Invalid(
					resourcesPath.Child("requests", "cpu"),
					requestCPU.String(),
					"CPU request cannot exceed CPU limit",
				))
			}

			requestMemory := instance.Spec.Resources.Requests[corev1.ResourceMemory]
			limitMemory := instance.Spec.Resources.Limits[corev1.ResourceMemory]
			if !limitMemory.IsZero() && requestMemory.Cmp(limitMemory) > 0 {
				allErrs = append(allErrs, field.Invalid(
					resourcesPath.Child("requests", "memory"),
					requestMemory.String(),
					"Memory request cannot exceed memory limit",
				))
			}
		}
	}

	// Validate image pull policy
	validPullPolicies := map[corev1.PullPolicy]bool{
		corev1.PullAlways:       true,
		corev1.PullIfNotPresent: true,
		corev1.PullNever:        true,
	}
	if instance.Spec.ImagePullPolicy != "" && !validPullPolicies[instance.Spec.ImagePullPolicy] {
		allErrs = append(allErrs, field.NotSupported(
			specPath.Child("imagePullPolicy"),
			instance.Spec.ImagePullPolicy,
			[]string{
				string(corev1.PullAlways),
				string(corev1.PullIfNotPresent),
				string(corev1.PullNever),
			},
		))
	}

	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "dbhub.tas.io", Kind: "DBHubInstance"},
			instance.Name,
			allErrs,
		)
	}

	return warnings, nil
}
