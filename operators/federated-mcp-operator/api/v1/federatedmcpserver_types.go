package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FederatedMCPServerSpec declares a downstream MCP server that the operator
// registers into the tas-mcp federation gateway. The fields mirror the
// gateway's registerable server model (federation.MCPServer); the controller
// maps this spec onto the gateway's POST /api/v1/federation/servers payload.
type FederatedMCPServerSpec struct {
	// ServerID is the stable federation id for this server. Defaults to the CR
	// name when empty. Changing it re-registers under the new id.
	// +optional
	ServerID string `json:"serverID,omitempty"`

	// DisplayName is the human-readable server name.
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// Description is a short description of what the server provides.
	// +optional
	Description string `json:"description,omitempty"`

	// Version is the server's semantic version.
	// +kubebuilder:default="1.0.0"
	Version string `json:"version,omitempty"`

	// Category groups the server (e.g. "search", "database", "development-tools").
	// +optional
	Category string `json:"category,omitempty"`

	// Endpoint is the URL the gateway uses to reach the server.
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`

	// Protocol is the transport the gateway speaks to the server.
	// +kubebuilder:validation:Enum=http;grpc;sse;stdio
	// +kubebuilder:default=http
	Protocol string `json:"protocol,omitempty"`

	// Auth describes how the gateway authenticates to the server.
	// +optional
	Auth AuthSpec `json:"auth,omitempty"`

	// Capabilities is the list of tools/methods the server exposes.
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`

	// Tags are free-form labels for discovery/filtering.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Metadata is arbitrary key/value metadata attached to the registration.
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`

	// Reduce opts this server's tool-call results into cache-safe
	// reduce-at-source at the gateway. Default false. Reduction drops
	// less-relevant chunks — suitable for text/RAG-heavy results, but it can lose
	// data on structured output (SQL rows, metadata), so enable it only for
	// servers whose results are safe to reduce.
	// +optional
	Reduce bool `json:"reduce,omitempty"`
}

// AuthSpec describes authentication to the downstream server. Credential VALUES
// are never inlined — they are read from the referenced Secret at reconcile time
// and forwarded to the gateway registration.
type AuthSpec struct {
	// Type is the auth method.
	// +kubebuilder:validation:Enum=none;api_key;oauth2;jwt;basic;bearer
	// +kubebuilder:default=none
	Type string `json:"type,omitempty"`

	// SecretRef references a Secret whose keys become the auth config map
	// forwarded to the gateway (e.g. {"api_key": "..."} or {"token": "..."}).
	// Ignored when Type is "none".
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// SecretReference points at a Secret in the same namespace as the CR.
type SecretReference struct {
	// Name of the Secret.
	Name string `json:"name"`
}

// FederatedMCPServerStatus is the observed registration state.
type FederatedMCPServerStatus struct {
	// Phase is the high-level lifecycle phase.
	// +kubebuilder:validation:Enum=Pending;Registered;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// Registered is true when the server is currently present in the gateway.
	// +optional
	Registered bool `json:"registered,omitempty"`

	// RegisteredID is the federation id under which the server was registered
	// (tracked so a ServerID change can unregister the old id).
	// +optional
	RegisteredID string `json:"registeredID,omitempty"`

	// ObservedGeneration is the .metadata.generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastRegisteredTime is when the server was last (re)registered.
	// +optional
	LastRegisteredTime *metav1.Time `json:"lastRegisteredTime,omitempty"`

	// LastError is the last reconcile error, if any.
	// +optional
	LastError string `json:"lastError,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.endpoint"
//+kubebuilder:printcolumn:name="Protocol",type="string",JSONPath=".spec.protocol"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Registered",type="boolean",JSONPath=".status.registered"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:resource:shortName=fms

// FederatedMCPServer is the Schema for the federatedmcpservers API. Each CR
// declares one downstream MCP server for the tas-mcp gateway to federate; the
// operator keeps the gateway's registry in sync with these CRs.
type FederatedMCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FederatedMCPServerSpec   `json:"spec,omitempty"`
	Status FederatedMCPServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FederatedMCPServerList contains a list of FederatedMCPServer.
type FederatedMCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FederatedMCPServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FederatedMCPServer{}, &FederatedMCPServerList{})
}
