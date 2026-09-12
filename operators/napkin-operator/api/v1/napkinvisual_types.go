package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NapkinVisualSpec defines the desired state of NapkinVisual
type NapkinVisualSpec struct {
	// Content is the text to visualize
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50000
	Content string `json:"content"`

	// Format is the output format
	// +kubebuilder:validation:Enum=svg;png;ppt
	// +kubebuilder:default=svg
	Format string `json:"format,omitempty"`

	// Style contains style configuration
	Style NapkinStyleSpec `json:"style,omitempty"`

	// Language is the BCP 47 language tag
	// +kubebuilder:default=en
	Language string `json:"language,omitempty"`

	// Variations is the number of variations to generate
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:default=1
	Variations int `json:"variations,omitempty"`

	// Context provides additional context for generation
	Context string `json:"context,omitempty"`

	// TenantId for multi-tenant isolation
	TenantId string `json:"tenantId,omitempty"`

	// ApiKeySecretRef references a Secret containing the Napkin API key
	ApiKeySecretRef SecretKeyRef `json:"apiKeySecretRef,omitempty"`

	// Storage configures where generated visuals are stored
	Storage NapkinStorageSpec `json:"storage,omitempty"`
}

// NapkinStyleSpec contains style configuration
type NapkinStyleSpec struct {
	// StyleId is the Napkin AI style identifier
	StyleId string `json:"styleId,omitempty"`

	// ColorMode is the color mode for generation
	// +kubebuilder:validation:Enum=light;dark;both
	// +kubebuilder:default=light
	ColorMode string `json:"colorMode,omitempty"`

	// Orientation controls the visual orientation
	// +kubebuilder:validation:Enum=auto;horizontal;vertical;square
	// +kubebuilder:default=auto
	Orientation string `json:"orientation,omitempty"`
}

// SecretKeyRef references a key in a Secret
type SecretKeyRef struct {
	// Name is the Secret name
	Name string `json:"name,omitempty"`

	// Key is the key within the Secret
	// +kubebuilder:default=NAPKIN_API_KEY
	Key string `json:"key,omitempty"`
}

// NapkinStorageSpec configures MinIO storage
type NapkinStorageSpec struct {
	// Bucket is the MinIO bucket name
	// +kubebuilder:default=napkin-visuals
	Bucket string `json:"bucket,omitempty"`

	// Prefix is the object key prefix
	Prefix string `json:"prefix,omitempty"`
}

// NapkinVisualStatus defines the observed state of NapkinVisual
type NapkinVisualStatus struct {
	// Phase is the current phase of the visual generation lifecycle
	// +kubebuilder:validation:Enum=Pending;Submitted;Processing;Downloading;Uploading;Completed;Failed
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations
	Conditions []NapkinVisualCondition `json:"conditions,omitempty"`

	// NapkinRequestId is the Napkin API request ID
	NapkinRequestId string `json:"napkinRequestId,omitempty"`

	// GeneratedFiles contains information about generated files
	GeneratedFiles []GeneratedFileStatus `json:"generatedFiles,omitempty"`

	// StartTime is when processing started
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when processing completed
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// RetryCount is the number of retries attempted
	RetryCount int `json:"retryCount,omitempty"`

	// LastError is the last error message
	LastError string `json:"lastError,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NapkinVisualCondition describes the state of a NapkinVisual at a certain point
type NapkinVisualCondition struct {
	// Type of condition
	// +kubebuilder:validation:Enum=Ready;Submitted;Downloaded;Uploaded
	Type string `json:"type"`

	// Status of the condition
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status string `json:"status"`

	// LastTransitionTime is the last time the condition transitioned
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a unique, one-word, CamelCase reason
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable message
	Message string `json:"message,omitempty"`
}

// GeneratedFileStatus contains information about a generated file
type GeneratedFileStatus struct {
	// Index of the file in the generation set
	Index int `json:"index"`

	// Format of the file
	Format string `json:"format"`

	// ColorMode used for this file
	ColorMode string `json:"colorMode,omitempty"`

	// NapkinUrl is the temporary Napkin download URL (expires in 30 min)
	NapkinUrl string `json:"napkinUrl,omitempty"`

	// MinioKey is the permanent MinIO object key
	MinioKey string `json:"minioKey,omitempty"`

	// MinioUrl is the permanent MinIO URL
	MinioUrl string `json:"minioUrl,omitempty"`

	// SizeBytes is the file size
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Format",type="string",JSONPath=".spec.format",description="Output format"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Current phase"
//+kubebuilder:printcolumn:name="Files",type="integer",JSONPath=".status.generatedFiles",description="Generated files count"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:resource:shortName=nv

// NapkinVisual is the Schema for the napkinvisuals API
type NapkinVisual struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NapkinVisualSpec   `json:"spec,omitempty"`
	Status NapkinVisualStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NapkinVisualList contains a list of NapkinVisual
type NapkinVisualList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NapkinVisual `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NapkinVisual{}, &NapkinVisualList{})
}
