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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DBHubInstancePhase defines the phase of the DBHub instance
// +kubebuilder:validation:Enum=Pending;Running;Failed;Degraded
type DBHubInstancePhase string

const (
	DBHubInstancePhasePending  DBHubInstancePhase = "Pending"
	DBHubInstancePhaseRunning  DBHubInstancePhase = "Running"
	DBHubInstancePhaseFailed   DBHubInstancePhase = "Failed"
	DBHubInstancePhaseDegraded DBHubInstancePhase = "Degraded"
)

// TransportType defines the transport protocol for DBHub
// +kubebuilder:validation:Enum=http;sse;stdio
type TransportType string

const (
	TransportTypeHTTP  TransportType = "http"
	TransportTypeSSE   TransportType = "sse"
	TransportTypeStdio TransportType = "stdio"
)

// DatabaseSelector defines how to select Database resources
type DatabaseSelector struct {
	// MatchLabels selects Database resources by labels
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// MatchNames selects Database resources by name
	// +optional
	MatchNames []string `json:"matchNames,omitempty"`
}

// DefaultPolicy defines the default access policy for databases
type DefaultPolicy struct {
	// ReadOnly restricts all databases to read-only operations
	// +kubebuilder:default=true
	// +optional
	ReadOnly bool `json:"readonly,omitempty"`

	// MaxRows limits the maximum number of rows returned by queries
	// +kubebuilder:default=1000
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100000
	// +optional
	MaxRows int32 `json:"maxRows,omitempty"`

	// AllowedOperations specifies which MCP tools are enabled
	// +kubebuilder:default={"execute_sql","search_objects"}
	// +optional
	AllowedOperations []string `json:"allowedOperations,omitempty"`
}

// ResourceRequirements defines the resource requests and limits
type ResourceRequirements struct {
	// Requests describes the minimum resources required
	// +optional
	Requests corev1.ResourceList `json:"requests,omitempty"`

	// Limits describes the maximum resources allowed
	// +optional
	Limits corev1.ResourceList `json:"limits,omitempty"`
}

// DBHubInstanceSpec defines the desired state of DBHubInstance
type DBHubInstanceSpec struct {
	// Replicas is the number of DBHub pods to run
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Image is the container image for DBHub
	// +kubebuilder:default="bytebase/dbhub:latest"
	// +optional
	Image string `json:"image,omitempty"`

	// ImagePullPolicy defines when to pull the container image
	// +kubebuilder:default=IfNotPresent
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Transport is the protocol used by DBHub (http, sse, stdio)
	// +kubebuilder:default=http
	// +optional
	Transport TransportType `json:"transport,omitempty"`

	// Port is the port number for DBHub to listen on
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// DatabaseSelector specifies which Database resources to include
	// +optional
	DatabaseSelector *DatabaseSelector `json:"databaseSelector,omitempty"`

	// DefaultPolicy sets default access policies for all databases
	// +optional
	DefaultPolicy *DefaultPolicy `json:"defaultPolicy,omitempty"`

	// Resources defines the resource requirements for the DBHub container
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// ServiceAccountName is the name of the ServiceAccount to use
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// NodeSelector is a selector for node assignment
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations are tolerations for pod scheduling
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity is the affinity rules for pod scheduling
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
}

// DBHubInstanceStatus defines the observed state of DBHubInstance
type DBHubInstanceStatus struct {
	// Phase represents the current state of the DBHub instance
	// +optional
	Phase DBHubInstancePhase `json:"phase,omitempty"`

	// AvailableReplicas is the number of ready replicas
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// ConnectedDatabases lists the names of connected Database resources
	// +optional
	ConnectedDatabases []string `json:"connectedDatabases,omitempty"`

	// Endpoint is the service endpoint for accessing DBHub
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// ConfigHash is a hash of the current configuration
	// +optional
	ConfigHash string `json:"configHash,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastConfigUpdate is the timestamp of the last configuration update
	// +optional
	LastConfigUpdate *metav1.Time `json:"lastConfigUpdate,omitempty"`

	// Conditions represent the current state of the DBHubInstance resource
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.availableReplicas
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas",description="Desired replicas"
// +kubebuilder:printcolumn:name="Available",type="integer",JSONPath=".status.availableReplicas",description="Available replicas"
// +kubebuilder:printcolumn:name="Databases",type="integer",JSONPath=".status.connectedDatabases",description="Connected databases"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Instance phase"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.endpoint",description="Service endpoint"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DBHubInstance is the Schema for the dbhubinstances API.
// It manages the deployment of DBHub MCP server instances that provide database access via the Model Context Protocol.
type DBHubInstance struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of DBHubInstance
	// +optional
	Spec DBHubInstanceSpec `json:"spec,omitempty"`

	// Status defines the observed state of DBHubInstance
	// +optional
	Status DBHubInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DBHubInstanceList contains a list of DBHubInstance
type DBHubInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DBHubInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DBHubInstance{}, &DBHubInstanceList{})
}

// GetReplicas returns the number of replicas, defaulting to 1 if not set
func (d *DBHubInstance) GetReplicas() int32 {
	if d.Spec.Replicas == nil {
		return 1
	}
	return *d.Spec.Replicas
}

// GetImage returns the container image, defaulting if not set
func (d *DBHubInstance) GetImage() string {
	if d.Spec.Image == "" {
		return "bytebase/dbhub:latest"
	}
	return d.Spec.Image
}

// GetPort returns the port, defaulting to 8080 if not set
func (d *DBHubInstance) GetPort() int32 {
	if d.Spec.Port == 0 {
		return 8080
	}
	return d.Spec.Port
}

// GetTransport returns the transport type, defaulting to http if not set
func (d *DBHubInstance) GetTransport() TransportType {
	if d.Spec.Transport == "" {
		return TransportTypeHTTP
	}
	return d.Spec.Transport
}

// MatchesDatabase returns true if the given Database matches the selector
func (d *DBHubInstance) MatchesDatabase(db *Database) bool {
	if d.Spec.DatabaseSelector == nil {
		// No selector means match all databases in the same namespace
		return db.Namespace == d.Namespace
	}

	// Check if database is in the same namespace
	if db.Namespace != d.Namespace {
		return false
	}

	// Check matchNames
	if len(d.Spec.DatabaseSelector.MatchNames) > 0 {
		found := false
		for _, name := range d.Spec.DatabaseSelector.MatchNames {
			if db.Name == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check matchLabels
	if len(d.Spec.DatabaseSelector.MatchLabels) > 0 {
		for key, value := range d.Spec.DatabaseSelector.MatchLabels {
			if db.Labels[key] != value {
				return false
			}
		}
	}

	return true
}
