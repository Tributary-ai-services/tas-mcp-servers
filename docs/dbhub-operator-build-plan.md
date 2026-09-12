# DBHub Operator Build Plan

> **Created:** January 2026
> **Status:** Planning
> **Branch:** `feature/dbhub-operator`
> **Design Reference:** [dbhub-operator-implementation.md](./dbhub-operator-implementation.md)
> **Pattern Reference:** [mcp-kubernetes-operator-pattern.md](./design-patterns/mcp-kubernetes-operator-pattern.md)
> **Integration Design:** [aether-dbhub-integration-design.md](./aether-dbhub-integration-design.md)

## Overview

Build a Kubernetes operator for DBHub that provides:
- **Database CRD** - Declarative database connection management
- **DBHubInstance CRD** - Automated DBHub deployment with database discovery
- **Multi-tenant support** - Namespace isolation with label-based selection
- **Secure credentials** - Integration with Kubernetes Secrets

## Project Structure

```
tas-mcp-servers/
├── operators/
│   └── dbhub-operator/
│       ├── api/
│       │   └── v1alpha1/
│       │       ├── database_types.go
│       │       ├── dbhubinstance_types.go
│       │       ├── groupversion_info.go
│       │       └── zz_generated.deepcopy.go
│       ├── cmd/
│       │   └── main.go
│       ├── config/
│       │   ├── crd/
│       │   │   └── bases/
│       │   │       ├── dbhub.tas.io_databases.yaml
│       │   │       └── dbhub.tas.io_dbhubinstances.yaml
│       │   ├── default/
│       │   ├── manager/
│       │   ├── rbac/
│       │   └── samples/
│       │       ├── database_postgres.yaml
│       │       ├── database_mysql.yaml
│       │       └── dbhubinstance.yaml
│       ├── internal/
│       │   └── controller/
│       │       ├── database_controller.go
│       │       ├── database_controller_test.go
│       │       ├── dbhubinstance_controller.go
│       │       ├── dbhubinstance_controller_test.go
│       │       └── suite_test.go
│       ├── Dockerfile
│       ├── Makefile
│       ├── PROJECT
│       ├── go.mod
│       └── README.md
├── k8s/
│   └── dbhub/           # Existing static manifests (keep for reference)
└── docs/
    └── ...              # Existing documentation
```

---

## Phase 1: Project Scaffolding

**Complexity:** 2 points | **Tasks:** 5 | **Risk:** Low
**Goal:** Initialize operator project with kubebuilder

### Tasks

- [ ] **1.1** Create `operators/dbhub-operator` directory
- [ ] **1.2** Initialize kubebuilder project
  ```bash
  kubebuilder init --domain tas.io --repo github.com/tas-io/dbhub-operator
  ```
- [ ] **1.3** Create Database API
  ```bash
  kubebuilder create api --group dbhub --version v1alpha1 --kind Database --resource --controller
  ```
- [ ] **1.4** Create DBHubInstance API
  ```bash
  kubebuilder create api --group dbhub --version v1alpha1 --kind DBHubInstance --resource --controller
  ```
- [ ] **1.5** Verify project builds
  ```bash
  make generate
  make manifests
  make build
  ```

### Deliverables
- Scaffolded operator project
- Generated CRD bases
- Compilable code

---

## Phase 2: CRD Implementation

**Complexity:** 5 points | **Tasks:** 8 | **Risk:** Low | **Depends on:** Phase 1
**Goal:** Define complete CRD schemas

### Tasks

#### 2.1 Database CRD

- [ ] **2.1.1** Define DatabaseSpec fields:
  ```go
  type DatabaseSpec struct {
      Type              string         `json:"type"`                        // postgres, mysql, mariadb, sqlserver, sqlite
      Host              string         `json:"host,omitempty"`
      Port              int            `json:"port,omitempty"`
      Database          string         `json:"database,omitempty"`
      CredentialsRef    CredentialsRef `json:"credentialsRef"`
      ConnectionTimeout int            `json:"connectionTimeout,omitempty"` // default: 30
      QueryTimeout      int            `json:"queryTimeout,omitempty"`      // default: 15
      SSLMode           string         `json:"sslMode,omitempty"`           // disable, require, verify-ca, verify-full
  }
  ```

- [ ] **2.1.2** Define DatabaseStatus fields:
  ```go
  type DatabaseStatus struct {
      Phase       string `json:"phase,omitempty"`       // Pending, Connected, Failed, Degraded
      LastChecked string `json:"lastChecked,omitempty"`
      Message     string `json:"message,omitempty"`
  }
  ```

- [ ] **2.1.3** Add kubebuilder markers for validation
- [ ] **2.1.4** Add printer columns for kubectl output

#### 2.2 DBHubInstance CRD

- [ ] **2.2.1** Define DBHubInstanceSpec fields:
  ```go
  type DBHubInstanceSpec struct {
      Replicas         int               `json:"replicas,omitempty"`
      Image            string            `json:"image,omitempty"`
      Transport        string            `json:"transport,omitempty"`        // http, sse
      Port             int               `json:"port,omitempty"`
      DatabaseSelector *DatabaseSelector `json:"databaseSelector,omitempty"`
      DefaultPolicy    *DefaultPolicy    `json:"defaultPolicy,omitempty"`
      Resources        *Resources        `json:"resources,omitempty"`
  }
  ```

- [ ] **2.2.2** Define DBHubInstanceStatus fields:
  ```go
  type DBHubInstanceStatus struct {
      Phase              string   `json:"phase,omitempty"`
      AvailableReplicas  int      `json:"availableReplicas,omitempty"`
      ConnectedDatabases []string `json:"connectedDatabases,omitempty"`
      Endpoint           string   `json:"endpoint,omitempty"`
  }
  ```

- [ ] **2.2.3** Add scale subresource
- [ ] **2.2.4** Add printer columns

#### 2.3 Generate and Verify

- [ ] **2.3.1** Run `make generate`
- [ ] **2.3.2** Run `make manifests`
- [ ] **2.3.3** Review generated CRD YAML files
- [ ] **2.3.4** Test CRD installation in cluster

### Deliverables
- Complete CRD schemas
- Generated YAML manifests
- CRDs installed in test cluster

---

## Phase 3: Database Controller

**Complexity:** 8 points | **Tasks:** 7 | **Risk:** Medium | **Depends on:** Phase 2
**Goal:** Implement controller for Database CR

### Tasks

- [ ] **3.1** Implement Reconcile loop
  - Fetch Database CR
  - Fetch credentials from Secret
  - Build DSN from spec + credentials
  - Test database connection
  - Update status

- [ ] **3.2** Implement DSN builder for each database type:
  ```go
  func buildDSN(db *Database, secret *corev1.Secret) string {
      switch db.Spec.Type {
      case "postgres":
          return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", ...)
      case "mysql", "mariadb":
          return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=%s", ...)
      case "sqlserver":
          return fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s", ...)
      }
  }
  ```

- [ ] **3.3** Implement connection tester:
  ```go
  func testConnection(dbType, dsn string) error {
      db, err := sql.Open(driverName, dsn)
      if err != nil {
          return err
      }
      defer db.Close()
      return db.Ping()
  }
  ```

- [ ] **3.4** Add database drivers to go.mod:
  - `github.com/lib/pq` (PostgreSQL)
  - `github.com/go-sql-driver/mysql` (MySQL/MariaDB)
  - `github.com/microsoft/go-mssqldb` (SQL Server)
  - `modernc.org/sqlite` (SQLite)

- [ ] **3.5** Implement periodic health checks (requeue after 5 min)

- [ ] **3.6** Write unit tests with mock database

- [ ] **3.7** Write integration tests with envtest

### Deliverables
- Working Database controller
- Connection testing for all DB types
- Unit and integration tests

---

## Phase 4: DBHubInstance Controller

**Complexity:** 13 points | **Tasks:** 9 | **Risk:** Medium | **Depends on:** Phase 3
**Goal:** Implement controller for DBHubInstance CR

### Tasks

#### 4.1 Resource Discovery

- [ ] **4.1.1** Implement database selector matching:
  ```go
  func findMatchingDatabases(instance *DBHubInstance) []Database {
      // List all Database CRs in namespace
      // Filter by matchLabels
      // Filter by matchNames
      // Return matching databases
  }
  ```

#### 4.2 Config Generation

- [ ] **4.2.1** Generate TOML config from databases:
  ```go
  func generateTOMLConfig(databases []Database, policy *DefaultPolicy) string {
      var toml strings.Builder
      for _, db := range databases {
          toml.WriteString(fmt.Sprintf(`
  [[sources]]
  id = "%s"
  dsn = "${%s_DSN}"
  connection_timeout = %d
  query_timeout = %d
  `, db.Name, envName(db.Name), db.Spec.ConnectionTimeout, db.Spec.QueryTimeout))
      }
      // Add tools section
      return toml.String()
  }
  ```

- [ ] **4.2.2** Create/update ConfigMap with TOML template

#### 4.3 Credential Aggregation

- [ ] **4.3.1** Aggregate DSNs from all matched databases:
  ```go
  func generateCredentialsSecret(instance *DBHubInstance, databases []Database) *corev1.Secret {
      data := make(map[string][]byte)
      for _, db := range databases {
          dsn := buildDSN(&db, getCredentials(&db))
          data[envName(db.Name)+"_DSN"] = []byte(dsn)
      }
      return &corev1.Secret{Data: data}
  }
  ```

#### 4.4 Deployment Management

- [ ] **4.4.1** Generate Deployment spec:
  - Init container for envsubst (config rendering)
  - Main container with DBHub image
  - Volume mounts for config
  - Resource limits from spec
  - Health probes

- [ ] **4.4.2** Implement create/update logic with server-side apply

#### 4.5 Service Management

- [ ] **4.5.1** Generate Service spec
- [ ] **4.5.2** Create/update Service

#### 4.6 Status Updates

- [ ] **4.6.1** Update status with:
  - Phase (Pending, Running, Failed, Degraded)
  - Available replicas
  - Connected databases list
  - Service endpoint

#### 4.7 Owner References

- [ ] **4.7.1** Set owner references on all created resources
- [ ] **4.7.2** Verify garbage collection works

#### 4.8 Watch Configuration

- [ ] **4.8.1** Watch owned Deployments
- [ ] **4.8.2** Watch owned Services
- [ ] **4.8.3** Watch owned ConfigMaps
- [ ] **4.8.4** Watch owned Secrets
- [ ] **4.8.5** Watch Database CRs (for re-reconcile on changes)

#### 4.9 Testing

- [ ] **4.9.1** Unit tests for config generation
- [ ] **4.9.2** Unit tests for credential aggregation
- [ ] **4.9.3** Integration tests with envtest

### Deliverables
- Working DBHubInstance controller
- Automatic database discovery
- Config and credential generation
- Full test coverage

---

## Phase 5: RBAC and Security

**Complexity:** 3 points | **Tasks:** 4 | **Risk:** Low | **Depends on:** Phase 4
**Goal:** Configure proper permissions

### Tasks

- [ ] **5.1** Define ClusterRole for operator:
  ```yaml
  - apiGroups: ["dbhub.tas.io"]
    resources: ["databases", "dbhubinstances"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["dbhub.tas.io"]
    resources: ["databases/status", "dbhubinstances/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets", "configmaps", "services"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  ```

- [ ] **5.2** Create ServiceAccount

- [ ] **5.3** Create ClusterRoleBinding

- [ ] **5.4** Test with restricted permissions

### Deliverables
- RBAC manifests
- Verified least-privilege access

---

## Phase 6: Build and Packaging

**Complexity:** 3 points | **Tasks:** 3 | **Risk:** Low | **Depends on:** Phase 5
**Goal:** Container image and Helm chart

### Tasks

- [ ] **6.1** Create multi-stage Dockerfile:
  ```dockerfile
  FROM golang:1.21 AS builder
  WORKDIR /workspace
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN CGO_ENABLED=0 go build -o manager cmd/main.go

  FROM gcr.io/distroless/static:nonroot
  COPY --from=builder /workspace/manager /manager
  USER 65532:65532
  ENTRYPOINT ["/manager"]
  ```

- [ ] **6.2** Build and push image:
  ```bash
  make docker-build IMG=registry.tas.scharber.com/dbhub-operator:v0.1.0
  make docker-push IMG=registry.tas.scharber.com/dbhub-operator:v0.1.0
  ```

- [ ] **6.3** Create Helm chart (optional):
  ```
  charts/dbhub-operator/
  ├── Chart.yaml
  ├── values.yaml
  └── templates/
      ├── deployment.yaml
      ├── rbac.yaml
      └── crds.yaml
  ```

### Deliverables
- Container image in registry
- Helm chart (optional)

---

## Phase 7: Deployment and Testing

**Complexity:** 5 points | **Tasks:** 7 | **Risk:** Medium | **Depends on:** Phase 6
**Goal:** Deploy to k3s and validate

### Tasks

- [ ] **7.1** Deploy CRDs:
  ```bash
  kubectl apply -f config/crd/bases/
  ```

- [ ] **7.2** Deploy operator:
  ```bash
  make deploy IMG=registry.tas.scharber.com/dbhub-operator:v0.1.0
  ```

- [ ] **7.3** Create test Database CRs:
  ```yaml
  apiVersion: dbhub.tas.io/v1alpha1
  kind: Database
  metadata:
    name: tas-postgres
    namespace: tas-mcp-servers
    labels:
      environment: production
  spec:
    type: postgres
    host: postgres-shared.tas-shared.svc.cluster.local
    port: 5432
    database: tas_shared
    credentialsRef:
      name: postgres-shared-secret
      namespace: tas-shared
      userKey: username
      passwordKey: password
    sslMode: disable
  ```

- [ ] **7.4** Create test DBHubInstance CR:
  ```yaml
  apiVersion: dbhub.tas.io/v1alpha1
  kind: DBHubInstance
  metadata:
    name: dbhub
    namespace: tas-mcp-servers
  spec:
    replicas: 1
    databaseSelector:
      matchLabels:
        environment: production
    defaultPolicy:
      readonly: true
      maxRows: 1000
  ```

- [ ] **7.5** Verify:
  - Database CR shows "Connected" status
  - DBHubInstance CR shows "Running" status
  - DBHub pod is running
  - MCP endpoint responds
  - Can query database via MCP

- [ ] **7.6** Test scaling:
  ```bash
  kubectl scale dbhubinstance dbhub -n tas-mcp-servers --replicas=2
  ```

- [ ] **7.7** Test database addition:
  - Add new Database CR
  - Verify DBHubInstance auto-discovers it
  - Verify config regenerated

### Deliverables
- Operator running in k3s
- End-to-end validation
- Documentation updates

---

## Phase 8: Documentation and Cleanup

**Complexity:** 2 points | **Tasks:** 6 | **Risk:** Low | **Depends on:** Phase 7
**Goal:** Complete documentation

### Tasks

- [ ] **8.1** Write operator README
- [ ] **8.2** Create usage examples
- [ ] **8.3** Document troubleshooting guide
- [ ] **8.4** Update main project README
- [ ] **8.5** Remove or archive static k8s/dbhub manifests
- [ ] **8.6** Final commit and PR

### Deliverables
- Complete documentation
- Clean repository
- PR ready for review

---

## Phase 9: Aether Integration

**Complexity:** 13 points | **Tasks:** 17 | **Risk:** Medium | **Depends on:** Phase 7
**Goal:** Integrate with Aether frontend and backend
**Design Reference:** [aether-dbhub-integration-design.md](./aether-dbhub-integration-design.md)

### Tasks

#### 9.1 Backend Integration (aether-be)

- [ ] **9.1.1** Create database models (`internal/models/database.go`)
  - Database, DatabaseCreateRequest, DatabaseUpdateRequest
  - QueryRequest, QueryResponse
  - SchemaResponse, TableInfo, ColumnInfo

- [ ] **9.1.2** Implement DatabaseService (`internal/services/database_service.go`)
  - CRUD operations with Neo4j storage
  - Kubernetes Secret management
  - Database CR lifecycle management
  - Status synchronization

- [ ] **9.1.3** Implement DBHubService (`internal/services/dbhub_service.go`)
  - MCP client for DBHub communication
  - Query execution via execute_sql tool
  - Schema retrieval via search_objects tool

- [ ] **9.1.4** Create DatabaseHandler (`internal/handlers/database_handler.go`)
  - REST API endpoints for database management
  - Query execution endpoint
  - Schema browser endpoint

- [ ] **9.1.5** Add route registration in `routes.go`
  - `/api/v1/databases` routes
  - Space context middleware

- [ ] **9.1.6** Add configuration for DBHub
  - DBHubConfig struct
  - Environment variables

- [ ] **9.1.7** Write unit tests for all components

#### 9.2 Frontend Integration (aether)

- [ ] **9.2.1** Create TypeScript types (`src/types/database.ts`)
  - Database, QueryResponse, SchemaResponse types
  - Request/response interfaces

- [ ] **9.2.2** Implement Redux slice (`src/store/slices/databaseSlice.ts`)
  - State management for databases, queries, schema
  - Async thunks for API calls

- [ ] **9.2.3** Create API service (`src/services/api/databaseApi.ts`)
  - API client methods for all endpoints

- [ ] **9.2.4** Build UI components:
  - DatabaseList - List of database connections
  - ConnectionForm - Create/edit database modal
  - QueryConsole - SQL editor with results
  - SchemaExplorer - Database schema browser

- [ ] **9.2.5** Add database page to routing
  - `/databases` - Database management
  - `/databases/:id/query` - Query console

- [ ] **9.2.6** Write component tests

#### 9.3 Integration Testing

- [ ] **9.3.1** End-to-end test: Create database via UI
- [ ] **9.3.2** End-to-end test: Execute query via UI
- [ ] **9.3.3** End-to-end test: Schema browsing
- [ ] **9.3.4** Test multi-tenant isolation
- [ ] **9.3.5** Test error handling and edge cases

### Deliverables
- Aether backend with database management API
- Aether frontend with database management UI
- Full integration with DBHub operator
- End-to-end test coverage

---

## Project Summary

| Phase | Complexity | Tasks | Risk | Dependencies |
|-------|------------|-------|------|--------------|
| 1. Scaffolding | 2 | 5 | Low | None |
| 2. CRD Implementation | 5 | 8 | Low | Phase 1 |
| 3. Database Controller | 8 | 7 | Medium | Phase 2 |
| 4. DBHubInstance Controller | 13 | 9 | Medium | Phase 3 |
| 5. RBAC | 3 | 4 | Low | Phase 4 |
| 6. Build/Packaging | 3 | 3 | Low | Phase 5 |
| 7. Deployment/Testing | 5 | 7 | Medium | Phase 6 |
| 8. Documentation | 2 | 6 | Low | Phase 7 |
| 9. Aether Integration | 13 | 17 | Medium | Phase 7 |

**Totals:** 54 complexity points | 66 tasks

### Complexity Scale (Fibonacci)
- **1-2:** Trivial, well-understood work
- **3-5:** Moderate complexity, some unknowns
- **8:** Significant complexity, multiple components
- **13:** High complexity, integration challenges
- **21+:** Should be broken down further

---

## Success Criteria

### Operator Criteria
- [ ] CRDs installed and validated
- [ ] Database CR can connect to PostgreSQL, MySQL
- [ ] DBHubInstance auto-discovers databases by labels
- [ ] Config automatically regenerated on database changes
- [ ] Credentials securely managed via Secrets
- [ ] Operator handles failures gracefully
- [ ] All tests passing
- [ ] Documentation complete

### Aether Integration Criteria
- [ ] Users can create database connections via Aether UI
- [ ] Users can execute SQL queries via query console
- [ ] Users can browse database schemas
- [ ] Multi-tenant isolation enforced (users see only their databases)
- [ ] Connection status displayed in real-time
- [ ] Credentials never exposed in frontend or logs
- [ ] Read-only mode enforced when configured
- [ ] Query history maintained per user

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| DBHub TOML format changes | Pin DBHub version, add version detection |
| Database driver compatibility | Test all supported databases |
| Secret access across namespaces | Document RBAC requirements |
| Config reload without restart | Use init container pattern |

---

## Next Steps

1. Create feature branch `feature/dbhub-operator`
2. Start Phase 1: Project scaffolding
3. Review and adjust plan as needed
