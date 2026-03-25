package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatasourceConnectionSpec defines the desired state of DatasourceConnection
type DatasourceConnectionSpec struct {
	// Type is the datasource type: neo4j, postgres, minio, kafka, grafana
	// +kubebuilder:validation:Enum=neo4j;postgres;minio;kafka;grafana
	Type string `json:"type"`

	// Host is the hostname or IP address
	Host string `json:"host"`

	// Port is the port number
	Port int `json:"port"`

	// Database is the database name (optional for some types)
	// +optional
	Database string `json:"database,omitempty"`

	// Protocol is the connection protocol (e.g., bolt, bolt+s, http, https)
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// SSLMode is the SSL mode (e.g., disable, require, verify-ca)
	// +optional
	SSLMode string `json:"sslMode,omitempty"`

	// CredentialsSecretRef is a reference to a K8s Secret containing username and password
	CredentialsSecretRef SecretRef `json:"credentialsSecretRef"`

	// TenantID is the tenant identifier for multi-tenant isolation
	// +optional
	TenantID string `json:"tenantId,omitempty"`

	// SpaceID is the space identifier
	// +optional
	SpaceID string `json:"spaceId,omitempty"`

	// OwnerUserID is the user who owns this connection
	// +optional
	OwnerUserID string `json:"ownerUserId,omitempty"`

	// BridgeEndpoint is an optional override for the MCP bridge URL
	// +optional
	BridgeEndpoint string `json:"bridgeEndpoint,omitempty"`

	// HealthCheckInterval is how often to re-test the connection (default 300s)
	// +kubebuilder:default=300
	// +optional
	HealthCheckInterval int `json:"healthCheckInterval,omitempty"`
}

// SecretRef references a Kubernetes Secret
type SecretRef struct {
	// Name is the Secret name
	Name string `json:"name"`

	// UsernameKey is the key in the Secret for the username (default: username)
	// +kubebuilder:default=username
	// +optional
	UsernameKey string `json:"usernameKey,omitempty"`

	// PasswordKey is the key in the Secret for the password (default: password)
	// +kubebuilder:default=password
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// DatasourceConnectionPhase represents the current phase
// +kubebuilder:validation:Enum=Pending;Testing;Connected;Failed
type DatasourceConnectionPhase string

const (
	PhasePending   DatasourceConnectionPhase = "Pending"
	PhaseTesting   DatasourceConnectionPhase = "Testing"
	PhaseConnected DatasourceConnectionPhase = "Connected"
	PhaseFailed    DatasourceConnectionPhase = "Failed"
)

// DatasourceConnectionStatus defines the observed state of DatasourceConnection
type DatasourceConnectionStatus struct {
	// Phase is the current lifecycle phase
	Phase DatasourceConnectionPhase `json:"phase,omitempty"`

	// LastTestedAt is the timestamp of the last health check
	// +optional
	LastTestedAt *metav1.Time `json:"lastTestedAt,omitempty"`

	// LastTestResult is the result of the last health check (success or error message)
	// +optional
	LastTestResult string `json:"lastTestResult,omitempty"`

	// LatencyMs is the latency of the last successful test in milliseconds
	// +optional
	LatencyMs int64 `json:"latencyMs,omitempty"`

	// Message is a human-readable description of the current state
	// +optional
	Message string `json:"message,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.host`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Latency",type=integer,JSONPath=`.status.latencyMs`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DatasourceConnection is the Schema for the datasourceconnections API
type DatasourceConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatasourceConnectionSpec   `json:"spec,omitempty"`
	Status DatasourceConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatasourceConnectionList contains a list of DatasourceConnection
type DatasourceConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatasourceConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatasourceConnection{}, &DatasourceConnectionList{})
}
