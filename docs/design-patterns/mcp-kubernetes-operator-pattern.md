# MCP Kubernetes Operator Design Pattern

> **Version:** 1.0
> **Status:** Approved
> **Applies To:** All MCP server deployments in TAS

## Overview

This document defines the standard design pattern for deploying MCP (Model Context Protocol) servers in Kubernetes using custom operators. This pattern provides declarative resource management, automatic discovery, multi-tenant isolation, and secure credential handling.

## Problem Statement

MCP servers typically require:
- Configuration at startup (connections, credentials, settings)
- Multiple instances for different environments/tenants
- Secure credential management
- Health monitoring and status reporting
- Dynamic reconfiguration without manual TOML/config file management

Traditional deployment approaches (static ConfigMaps, manual secrets) don't scale well for multi-tenant or multi-environment scenarios.

## Solution: Kubernetes Operator Pattern

Use Custom Resource Definitions (CRDs) and controllers to:
1. **Declaratively define resources** (databases, APIs, services)
2. **Auto-discover and aggregate** resources into MCP server configs
3. **Manage credentials securely** via Secret references
4. **Monitor health** and report status on CRs
5. **Enable multi-tenancy** via namespace isolation and label selectors

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌────────────────────┐                                             │
│  │   MCP Operator     │                                             │
│  │                    │                                             │
│  │  ┌──────────────┐  │     ┌─────────────────────────────────┐    │
│  │  │  Resource    │──┼────▶│      Tenant Namespace            │    │
│  │  │  Controller  │  │     │                                  │    │
│  │  └──────────────┘  │     │  ┌──────────┐  ┌──────────┐     │    │
│  │                    │     │  │Resource  │  │Resource  │     │    │
│  │  ┌──────────────┐  │     │  │  CR #1   │  │  CR #2   │     │    │
│  │  │  Instance    │──┼────▶│  └────┬─────┘  └────┬─────┘     │    │
│  │  │  Controller  │  │     │       │              │          │    │
│  │  └──────────────┘  │     │       ▼              ▼          │    │
│  │                    │     │  ┌─────────────────────────┐    │    │
│  └────────────────────┘     │  │   MCPServerInstance     │    │    │
│                             │  │                         │    │    │
│                             │  │  ┌───────┐ ┌───────┐   │    │    │
│                             │  │  │ Pod   │ │ Pod   │   │    │    │
│                             │  │  └───────┘ └───────┘   │    │    │
│                             │  └─────────────────────────┘    │    │
│                             │                                  │    │
│                             └─────────────────────────────────┘    │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Generic CRD Templates

### Resource CRD (Abstract)

Every MCP server type needs a "Resource" CRD that represents the external service/data source it connects to.

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: <resources>.<group>.tas.io
spec:
  group: <group>.tas.io
  names:
    kind: <Resource>
    listKind: <Resource>List
    plural: <resources>
    singular: <resource>
    shortNames:
      - <short>
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
                # Resource-specific connection fields
                type:
                  type: string
                  description: "Type of resource"
                credentialsRef:
                  type: object
                  description: "Reference to credentials Secret"
                  properties:
                    name:
                      type: string
                    namespace:
                      type: string
                # Add resource-specific fields here
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
      subresources:
        status: {}
```

### MCPServerInstance CRD (Abstract)

Every MCP server type needs an "Instance" CRD that manages the deployment.

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: <mcp>instances.<group>.tas.io
spec:
  group: <group>.tas.io
  names:
    kind: <MCP>Instance
    listKind: <MCP>InstanceList
    plural: <mcp>instances
    singular: <mcp>instance
    shortNames:
      - <short>i
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
                image:
                  type: string
                  description: "Container image for MCP server"
                transport:
                  type: string
                  enum: ["http", "sse", "stdio"]
                  default: "http"
                port:
                  type: integer
                  default: 8080
                resourceSelector:
                  type: object
                  description: "Selector for resources to include"
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
                  description: "Default access policy"
                resources:
                  type: object
                  description: "Pod resource requests/limits"
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: ["Pending", "Running", "Failed", "Degraded"]
                availableReplicas:
                  type: integer
                connectedResources:
                  type: array
                  items:
                    type: string
                endpoint:
                  type: string
      subresources:
        status: {}
        scale:
          specReplicasPath: .spec.replicas
          statusReplicasPath: .status.availableReplicas
```

## Controller Pattern

### Resource Controller Responsibilities

1. **Watch** Resource CRs in all namespaces (or configured namespaces)
2. **Fetch credentials** from referenced Secrets
3. **Validate connection** to external service
4. **Update status** with connection health
5. **Requeue** for periodic health checks

```go
func (r *ResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch the Resource CR
    // 2. Fetch credentials from Secret
    // 3. Test connection to external service
    // 4. Update status (Connected/Failed)
    // 5. Requeue for health check interval
    return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}
```

### Instance Controller Responsibilities

1. **Watch** Instance CRs
2. **Find matching Resources** via label/name selector
3. **Generate configuration** (TOML, JSON, YAML) from Resources
4. **Create/update ConfigMap** with generated config
5. **Aggregate credentials** into single Secret
6. **Create/update Deployment** with config mounted
7. **Create/update Service** for network access
8. **Update status** with endpoint and connected resources

```go
func (r *InstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch the Instance CR
    // 2. Find matching Resource CRs
    // 3. Generate config (ConfigMap)
    // 4. Aggregate credentials (Secret)
    // 5. Create/update Deployment
    // 6. Create/update Service
    // 7. Update status
    return ctrl.Result{}, nil
}
```

## Init Container Pattern for Config Rendering

Use an init container to render environment variables into config files:

```yaml
initContainers:
  - name: config-renderer
    image: bhgedigital/envsubst:latest
    command: ["sh", "-c", "envsubst < /config-template/config.toml > /config/config.toml"]
    envFrom:
      - secretRef:
          name: <instance>-creds
    volumeMounts:
      - name: config-template
        mountPath: /config-template
      - name: config-rendered
        mountPath: /config
```

This allows:
- ConfigMap contains template with `${VAR_NAME}` placeholders
- Secret contains actual credential values
- Init container renders final config with credentials
- Main container reads rendered config (no credentials in ConfigMap)

## Multi-Tenancy Pattern

### Namespace Isolation

Each tenant gets their own namespace:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: tas-<tenant>
  labels:
    tas.io/tenant: <tenant>
```

### Resource Scoping

Resources are namespaced - each tenant defines their own:

```yaml
apiVersion: dbhub.tas.io/v1alpha1
kind: Database
metadata:
  name: analytics
  namespace: tas-acme  # Tenant namespace
  labels:
    environment: production
```

### Instance Scoping

Instances discover resources within their namespace only:

```go
// Find resources in same namespace as instance
r.List(ctx, &resources, client.InNamespace(instance.Namespace))
```

### Cross-Namespace Resources (Optional)

For shared resources, use `credentialsRef.namespace`:

```yaml
credentialsRef:
  name: shared-db-creds
  namespace: tas-shared  # Different namespace
```

## External Secrets Integration

Integrate with External Secrets Operator for production credential management:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: <resource>-creds
  namespace: tas-<tenant>
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: <resource>-creds
  data:
    - secretKey: username
      remoteRef:
        key: tas/<tenant>/<resource>
        property: username
    - secretKey: password
      remoteRef:
        key: tas/<tenant>/<resource>
        property: password
```

## Standard Labels

All resources should include these labels:

```yaml
metadata:
  labels:
    # Required
    app.kubernetes.io/name: <resource-name>
    app.kubernetes.io/instance: <instance-name>
    app.kubernetes.io/component: <mcp-type>
    app.kubernetes.io/part-of: tas-mcp-servers

    # Recommended
    app.kubernetes.io/version: <version>
    app.kubernetes.io/managed-by: <operator-name>

    # TAS-specific
    tas.io/tenant: <tenant>
    tas.io/environment: <env>
```

## Operator Project Structure

```
<mcp>-operator/
├── api/
│   └── v1alpha1/
│       ├── <resource>_types.go
│       ├── <mcp>instance_types.go
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
├── cmd/
│   └── main.go
├── config/
│   ├── crd/
│   │   └── bases/
│   ├── default/
│   ├── manager/
│   ├── rbac/
│   └── samples/
├── internal/
│   └── controller/
│       ├── <resource>_controller.go
│       └── <mcp>instance_controller.go
├── Dockerfile
├── Makefile
├── PROJECT
└── go.mod
```

## Applying This Pattern

### For DBHub (Database MCP)

| Generic | DBHub-Specific |
|---------|----------------|
| Resource | Database |
| MCPServerInstance | DBHubInstance |
| Group | dbhub.tas.io |
| Config format | TOML |

### For Future MCP Servers

| MCP Server | Resource CR | Instance CR | Group |
|------------|-------------|-------------|-------|
| DBHub | Database | DBHubInstance | dbhub.tas.io |
| Search MCP | SearchEngine | SearchMCPInstance | search.tas.io |
| Storage MCP | StorageBucket | StorageMCPInstance | storage.tas.io |
| API MCP | APIEndpoint | APIMCPInstance | api.tas.io |

## Implementation Checklist

- [ ] Define Resource CRD with connection fields
- [ ] Define Instance CRD with selector and policy fields
- [ ] Implement Resource controller with health checks
- [ ] Implement Instance controller with config generation
- [ ] Create RBAC roles for controller
- [ ] Write unit tests for controllers
- [ ] Write integration tests with envtest
- [ ] Create example CRs for testing
- [ ] Document usage in README
- [ ] Build and push operator image
- [ ] Deploy to cluster

## References

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Operator SDK](https://sdk.operatorframework.io/)
- [External Secrets Operator](https://external-secrets.io/)
- [MCP Specification](https://modelcontextprotocol.io/specification/)
