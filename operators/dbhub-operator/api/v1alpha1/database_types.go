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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseType defines the type of database
// +kubebuilder:validation:Enum=postgres;mysql;mariadb;sqlserver;sqlite
type DatabaseType string

const (
	DatabaseTypePostgres  DatabaseType = "postgres"
	DatabaseTypeMySQL     DatabaseType = "mysql"
	DatabaseTypeMariaDB   DatabaseType = "mariadb"
	DatabaseTypeSQLServer DatabaseType = "sqlserver"
	DatabaseTypeSQLite    DatabaseType = "sqlite"
)

// DatabasePhase defines the phase of the database connection
// +kubebuilder:validation:Enum=Pending;Connected;Failed;Degraded
type DatabasePhase string

const (
	DatabasePhasePending   DatabasePhase = "Pending"
	DatabasePhaseConnected DatabasePhase = "Connected"
	DatabasePhaseFailed    DatabasePhase = "Failed"
	DatabasePhaseDegraded  DatabasePhase = "Degraded"
)

// SSLMode defines the SSL mode for database connections
// +kubebuilder:validation:Enum=disable;require;verify-ca;verify-full
type SSLMode string

const (
	SSLModeDisable    SSLMode = "disable"
	SSLModeRequire    SSLMode = "require"
	SSLModeVerifyCA   SSLMode = "verify-ca"
	SSLModeVerifyFull SSLMode = "verify-full"
)

// CredentialsRef references a Kubernetes Secret containing database credentials
type CredentialsRef struct {
	// Name of the Secret containing credentials
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the Secret. Defaults to the Database's namespace if not specified.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key in the Secret containing the username
	// +kubebuilder:default=username
	// +optional
	UserKey string `json:"userKey,omitempty"`

	// Key in the Secret containing the password
	// +kubebuilder:default=password
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// DatabaseSpec defines the desired state of Database
type DatabaseSpec struct {
	// Type of database (postgres, mysql, mariadb, sqlserver, sqlite)
	// +kubebuilder:validation:Required
	Type DatabaseType `json:"type"`

	// Host is the database server hostname or IP address
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Port is the database server port
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Database is the name of the database to connect to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Database string `json:"database"`

	// CredentialsRef references the Secret containing database credentials
	// +kubebuilder:validation:Required
	CredentialsRef CredentialsRef `json:"credentialsRef"`

	// ConnectionTimeout is the maximum time in seconds to wait for a connection
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	// +optional
	ConnectionTimeout int32 `json:"connectionTimeout,omitempty"`

	// QueryTimeout is the maximum time in seconds to wait for a query to complete
	// +kubebuilder:default=15
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	// +optional
	QueryTimeout int32 `json:"queryTimeout,omitempty"`

	// SSLMode configures SSL/TLS for the connection
	// +kubebuilder:default=disable
	// +optional
	SSLMode SSLMode `json:"sslMode,omitempty"`

	// MaxRows limits the maximum number of rows returned by queries
	// +kubebuilder:default=1000
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100000
	// +optional
	MaxRows int32 `json:"maxRows,omitempty"`

	// ReadOnly restricts the database to read-only operations
	// +kubebuilder:default=true
	// +optional
	ReadOnly bool `json:"readOnly,omitempty"`

	// Description is a human-readable description of the database
	// +kubebuilder:validation:MaxLength=1000
	// +optional
	Description string `json:"description,omitempty"`
}

// DatabaseStatus defines the observed state of Database
type DatabaseStatus struct {
	// Phase represents the current state of the database connection
	// +optional
	Phase DatabasePhase `json:"phase,omitempty"`

	// LastChecked is the timestamp of the last connection check
	// +optional
	LastChecked *metav1.Time `json:"lastChecked,omitempty"`

	// Message provides additional information about the current phase
	// +optional
	Message string `json:"message,omitempty"`

	// DSN is the constructed data source name (without credentials)
	// +optional
	DSN string `json:"dsn,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the current state of the Database resource
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="Database type"
// +kubebuilder:printcolumn:name="Host",type="string",JSONPath=".spec.host",description="Database host"
// +kubebuilder:printcolumn:name="Port",type="integer",JSONPath=".spec.port",description="Database port"
// +kubebuilder:printcolumn:name="Database",type="string",JSONPath=".spec.database",description="Database name"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Connection phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Database is the Schema for the databases API.
// It represents a database connection configuration that DBHub can use to connect to external databases.
type Database struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of Database
	// +kubebuilder:validation:Required
	Spec DatabaseSpec `json:"spec"`

	// Status defines the observed state of Database
	// +optional
	Status DatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseList contains a list of Database
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}

// GetCredentialsNamespace returns the namespace for the credentials Secret
func (d *Database) GetCredentialsNamespace() string {
	if d.Spec.CredentialsRef.Namespace != "" {
		return d.Spec.CredentialsRef.Namespace
	}
	return d.Namespace
}

// GetUserKey returns the key for the username in the credentials Secret
func (d *Database) GetUserKey() string {
	if d.Spec.CredentialsRef.UserKey != "" {
		return d.Spec.CredentialsRef.UserKey
	}
	return "username"
}

// GetPasswordKey returns the key for the password in the credentials Secret
func (d *Database) GetPasswordKey() string {
	if d.Spec.CredentialsRef.PasswordKey != "" {
		return d.Spec.CredentialsRef.PasswordKey
	}
	return "password"
}
