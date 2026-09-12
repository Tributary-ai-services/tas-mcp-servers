// Package v1 contains API Schema definitions for the mcp v1 API group,
// specifically the FederatedMCPServer custom resource that declares a downstream
// MCP server to register into the tas-mcp federation gateway.
// +kubebuilder:object:generate=true
// +groupName=mcp.tas.ai
package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "mcp.tas.ai", Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
