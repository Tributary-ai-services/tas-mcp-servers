# DBHub Operator

A Kubernetes operator for managing DBHub SQL MCP (Model Context Protocol) servers. This operator enables declarative management of database connections and DBHub instances in Kubernetes environments.

## Overview

The DBHub Operator introduces two Custom Resources:

- **Database**: Represents a database connection with credentials and configuration
- **DBHubInstance**: Represents a DBHub server instance that connects to selected databases

## Features

- Declarative database connection management
- Automatic credential handling via Kubernetes Secrets
- Dynamic configuration generation for DBHub servers
- Health checking and connection monitoring
- Selector-based database grouping
- Automatic resource management (Deployment, Service, ConfigMap, Secret)
- Webhook validation and defaulting
- Helm chart for easy deployment

## Installation

### Prerequisites

- Kubernetes cluster (v1.24+)
- kubectl configured to access your cluster
- cert-manager installed (for webhook certificates)

### Using Helm

```bash
# Install the operator
helm install dbhub-operator ./helm/dbhub-operator \
  --namespace dbhub-operator-system \
  --create-namespace
```

### Using Kustomize

```bash
# Install CRDs
kubectl apply -f config/crd/bases/

# Deploy the operator
kubectl apply -k config/default/
```

### From Source

```bash
# Build and push the image
make docker-build docker-push IMG=<your-registry>/dbhub-operator:latest

# Deploy
make deploy IMG=<your-registry>/dbhub-operator:latest
```

## Usage

### Creating a Database Connection

First, create a Kubernetes Secret with database credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-credentials
  namespace: default
type: Opaque
data:
  username: cG9zdGdyZXM=    # base64 encoded
  password: c2VjcmV0MTIz    # base64 encoded
```

Then create a Database resource:

```yaml
apiVersion: dbhub.tas.io/v1alpha1
kind: Database
metadata:
  name: my-postgres
  namespace: default
  labels:
    environment: production
    team: platform
spec:
  type: postgres
  host: postgres.database.svc.cluster.local
  port: 5432
  database: myapp
  credentialsRef:
    name: postgres-credentials
    userKey: username
    passwordKey: password
  sslMode: require
  connectionTimeout: 30
  queryTimeout: 60
  readOnly: true
  maxRows: 1000
  description: "Production PostgreSQL database"
```

### Creating a DBHub Instance

Create a DBHubInstance that selects databases by label:

```yaml
apiVersion: dbhub.tas.io/v1alpha1
kind: DBHubInstance
metadata:
  name: dbhub-production
  namespace: default
spec:
  replicas: 2
  image: bytebase/dbhub:latest
  transport: http
  port: 8080

  # Select databases by labels
  databaseSelector:
    matchLabels:
      environment: production

  # Or select by name
  # databaseSelector:
  #   matchNames:
  #     - my-postgres
  #     - my-mysql

  defaultPolicy:
    readonly: true
    maxRows: 1000
    allowedOperations:
      - execute_sql
      - search_objects

  resources:
    requests:
      memory: "128Mi"
      cpu: "100m"
    limits:
      memory: "512Mi"
      cpu: "500m"
```

## CRD Reference

### Database

| Field | Type | Description |
|-------|------|-------------|
| `spec.type` | string | Database type: `postgres`, `mysql`, `mariadb`, `sqlserver`, `sqlite` |
| `spec.host` | string | Database hostname |
| `spec.port` | int32 | Database port |
| `spec.database` | string | Database name |
| `spec.credentialsRef.name` | string | Name of the Secret containing credentials |
| `spec.credentialsRef.namespace` | string | Namespace of the Secret (optional, defaults to Database namespace) |
| `spec.credentialsRef.userKey` | string | Key for username in Secret (default: `username`) |
| `spec.credentialsRef.passwordKey` | string | Key for password in Secret (default: `password`) |
| `spec.sslMode` | string | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |
| `spec.connectionTimeout` | int32 | Connection timeout in seconds |
| `spec.queryTimeout` | int32 | Query timeout in seconds |
| `spec.readOnly` | bool | Enable read-only mode |
| `spec.maxRows` | int32 | Maximum rows returned per query |
| `spec.description` | string | Human-readable description |

### Database Status

| Field | Type | Description |
|-------|------|-------------|
| `status.phase` | string | Current phase: `Pending`, `Connected`, `Failed`, `Degraded` |
| `status.lastChecked` | timestamp | Last connection check time |
| `status.message` | string | Status message |
| `status.dsn` | string | DSN (without credentials) |
| `status.conditions` | []Condition | Detailed conditions |

### DBHubInstance

| Field | Type | Description |
|-------|------|-------------|
| `spec.replicas` | int32 | Number of replicas (default: 1) |
| `spec.image` | string | Container image (default: `bytebase/dbhub:latest`) |
| `spec.imagePullPolicy` | string | Image pull policy |
| `spec.transport` | string | Transport type: `http`, `sse`, `stdio` |
| `spec.port` | int32 | Service port (default: 8080) |
| `spec.databaseSelector.matchLabels` | map | Select databases by labels |
| `spec.databaseSelector.matchNames` | []string | Select databases by name |
| `spec.defaultPolicy.readonly` | bool | Enable read-only mode |
| `spec.defaultPolicy.maxRows` | int32 | Maximum rows per query |
| `spec.defaultPolicy.allowedOperations` | []string | Allowed MCP operations |
| `spec.resources` | ResourceRequirements | Container resources |
| `spec.nodeSelector` | map | Node selector |
| `spec.tolerations` | []Toleration | Pod tolerations |
| `spec.affinity` | Affinity | Pod affinity rules |
| `spec.serviceAccountName` | string | Service account to use |

### DBHubInstance Status

| Field | Type | Description |
|-------|------|-------------|
| `status.phase` | string | Current phase: `Pending`, `Running`, `Failed`, `Degraded` |
| `status.availableReplicas` | int32 | Number of available replicas |
| `status.endpoint` | string | Service endpoint |
| `status.connectedDatabases` | []string | List of connected database names |
| `status.configHash` | string | Hash of current configuration |
| `status.lastConfigUpdate` | timestamp | Last configuration update time |
| `status.conditions` | []Condition | Detailed conditions |

## Architecture

```
                          ┌─────────────────────┐
                          │   Kubernetes API    │
                          └──────────┬──────────┘
                                     │
              ┌──────────────────────┼──────────────────────┐
              │                      │                      │
              ▼                      ▼                      ▼
      ┌───────────────┐    ┌─────────────────┐    ┌───────────────┐
      │   Database    │    │  DBHubInstance  │    │    Secret     │
      │      CR       │    │       CR        │    │ (credentials) │
      └───────┬───────┘    └────────┬────────┘    └───────┬───────┘
              │                     │                     │
              │    ┌────────────────┼────────────────┐    │
              │    │                │                │    │
              ▼    ▼                ▼                ▼    ▼
      ┌────────────────────────────────────────────────────────┐
      │                  DBHub Operator                        │
      │  ┌──────────────────┐    ┌───────────────────────┐    │
      │  │ DatabaseReconciler│   │ DBHubInstanceReconciler│    │
      │  └──────────────────┘    └───────────────────────┘    │
      └────────────────────────────┬───────────────────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                    │
              ▼                    ▼                    ▼
      ┌───────────────┐    ┌───────────────┐    ┌───────────────┐
      │  Deployment   │    │   Service     │    │  ConfigMap    │
      │  (DBHub pods) │    │  (ClusterIP)  │    │  (TOML config)│
      └───────────────┘    └───────────────┘    └───────────────┘
```

## Development

### Building

```bash
# Build the binary
make build

# Build the Docker image
make docker-build IMG=<your-registry>/dbhub-operator:latest

# Run tests
make test

# Generate CRDs and RBAC
make manifests
```

### Running Locally

```bash
# Install CRDs
make install

# Run the operator locally
make run
```

### Code Generation

```bash
# Generate DeepCopy methods
make generate

# Generate CRD manifests
make manifests
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ENABLE_WEBHOOKS` | Enable admission webhooks | `true` |
| `LEADER_ELECTION` | Enable leader election | `false` |
| `METRICS_BIND_ADDRESS` | Metrics endpoint address | `:8443` |
| `HEALTH_PROBE_BIND_ADDRESS` | Health probe address | `:8081` |

### Webhook Configuration

Webhooks require cert-manager for TLS certificate management. The operator includes:

- **Mutating Webhook**: Sets default values for Database and DBHubInstance resources
- **Validating Webhook**: Validates resource specifications

## Troubleshooting

### Database Connection Failures

1. Check the Database status:
   ```bash
   kubectl get database my-postgres -o yaml
   ```

2. Verify the Secret exists and has correct keys:
   ```bash
   kubectl get secret postgres-credentials -o yaml
   ```

3. Check operator logs:
   ```bash
   kubectl logs -n dbhub-operator-system deployment/dbhub-operator-controller-manager
   ```

### DBHub Instance Not Starting

1. Check the DBHubInstance status:
   ```bash
   kubectl get dbhubinstance dbhub-production -o yaml
   ```

2. Verify matching databases exist:
   ```bash
   kubectl get database -l environment=production
   ```

3. Check the generated Deployment:
   ```bash
   kubectl get deployment dbhub-production -o yaml
   ```

## To Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## License

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
