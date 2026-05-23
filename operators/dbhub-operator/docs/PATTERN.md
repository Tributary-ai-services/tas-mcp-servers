# DBHub Operator Pattern

> **Source location note:** in the `main` checkout this directory contains only
> build artifacts (`bin/`, `cover.out`). The operator source lives on the
> `feature/neo4j-datasource` branch (commit `8cc377c`). Read it with
> `git show feature/neo4j-datasource:operators/dbhub-operator/<path>`.

This is the **most complete operator in TAS** — a full operator-sdk / kubebuilder
scaffold that *deploys and manages* DBHub MCP server instances and the database
connections they expose. Contrast with `napkin-operator` (external task operator)
and `datasource-operator` (connection-health operator) — see
`operators/OPERATOR-PATTERNS.md` for how the three relate.

## Stack

Go 1.25 · `sigs.k8s.io/controller-runtime` · `k8s.io/*` v0.35.0 · admission
webhooks + cert-manager · Prometheus monitor · Helm chart · envtest/ginkgo e2e.
Real SQL drivers: `lib/pq` (postgres), `go-sql-driver/mysql`, `microsoft/go-mssqldb`,
`modernc.org/sqlite`.

## Two custom resources (`dbhub.tas.io/v1alpha1`)

### `Database` — a database connection definition
Spec: `type` (postgres/mysql/mariadb/sqlserver/sqlite), `host`, `port`,
`database`, `credentialsRef` (Secret + user/password keys), `sslMode`,
`connectionTimeout`, `queryTimeout`, `maxRows`, `readOnly`. Status: `phase`
(Pending/Connected/Failed/Degraded), `lastChecked`, `dsn` (credential-free),
`conditions`, `observedGeneration`.

### `DBHubInstance` — a desired DBHub MCP server
Spec: `replicas`, `image` (default `bytebase/dbhub:latest`), `transport`
(http/sse/stdio), `port`, `databaseSelector` (matchLabels/matchNames),
`defaultPolicy` (readonly, maxRows, allowedOperations), `resources`, scheduling
(nodeSelector/tolerations/affinity). Status: `phase`, `availableReplicas`,
`connectedDatabases[]`, `endpoint`, `configHash`, `conditions`. Carries
`+kubebuilder:subresource:status` **and** `+kubebuilder:subresource:scale`
(so `kubectl scale dbhubinstance` works).

## Controller 1: `DatabaseReconciler` (connectivity prober)

`internal/controller/database_controller.go`. On each reconcile it reads the
referenced Secret, builds a driver-specific DSN, **opens a real SQL connection**
to test reachability, and writes `phase` + a `Connected` condition (via
`meta.SetStatusCondition`). It requeues every `HealthCheckInterval` (5m) so the
status reflects live connectivity. RBAC: `databases` + `databases/status` +
`secrets` (read).

## Controller 2: `DBHubInstanceReconciler` (the deployment operator)

`internal/controller/dbhubinstance_controller.go` — the textbook "owns child
objects" pattern:

1. **Select inputs** — `findMatchingDatabases()` lists `Database` CRs in the
   namespace, filters by the instance's selector, and keeps only those in phase
   `Connected`.
2. **Render config** — `generateConfig()` builds a DBHub `dbhub.toml`
   (`[[sources]]` per database with `dsn = "${NAME_DSN}"` placeholders, plus
   `[[tools]]` execute_sql/search_objects honoring `defaultPolicy`) and a map of
   `NAME_DSN` → real DSN pulled from each database's Secret.
3. **Compute `configHash`** (sha256) for change detection.
4. **Reconcile child objects**, each via create-or-update with
   `controllerutil.SetControllerReference(instance, …)`:
   - `ConfigMap` (`<name>-config`) holding the TOML template
   - `Secret` (`<name>-creds`) holding the DSNs
   - `Deployment` — an **initContainer** (`envsubst`) renders the template +
     DSN secret into an `emptyDir`, the `dbhub` container then runs against the
     rendered config; the `config-hash` label on the pod template forces a
     rollout when config changes
   - `Service` (ClusterIP, preserves existing ClusterIP on update)
5. **Roll up status** — set `endpoint`, read the child Deployment's
   `availableReplicas`, map to phase Running/Degraded/Pending, set conditions.

Watch wiring (`SetupWithManager`):

```go
ctrl.NewControllerManagedBy(mgr).
    For(&dbhubv1alpha1.DBHubInstance{}).
    Owns(&appsv1.Deployment{}).
    Owns(&corev1.Service{}).
    Owns(&corev1.ConfigMap{}).
    Owns(&corev1.Secret{}).
    Watches(&dbhubv1alpha1.Database{},
        handler.EnqueueRequestsFromMapFunc(r.findInstancesForDatabase)).
    Named("dbhubinstance").
    Complete(r)
```

`Owns(...)` gives automatic garbage collection of children (via owner refs) and
re-reconcile on child drift; the `Watches(Database, …)` map function re-queues
any instance whose selector matches a changed `Database`, so adding/connecting a
database automatically reconfigures and rolls the DBHub pods.

## Admission webhooks

`internal/webhook/v1alpha1/` registers a **mutating defaulter** and a
**validating webhook** for both kinds (e.g. `Database` validates host/port/db
name, requires credentials for non-SQLite, warns on type changes). Marker
example:

```go
// +kubebuilder:webhook:path=/validate-dbhub-tas-io-v1alpha1-database,mutating=false,
//   failurePolicy=fail,groups=dbhub.tas.io,resources=databases,verbs=create;update,...
```

Webhook serving certs are wired through cert-manager (`config/certmanager/`),
and webhooks are only set up when `ENABLE_WEBHOOKS != "false"`.

## Manager & project layout

`cmd/main.go` is the standard kubebuilder entrypoint: registers both reconcilers,
optional webhook setup, secure metrics server (optional cert watcher),
health/ready probes, leader election (`LeaderElectionID: 06aaabf9.tas.io`). The
project ships the full kubebuilder `config/` kustomize tree (crd, rbac with
admin/editor/viewer roles, certmanager, network-policy, prometheus, webhook,
samples), a **Helm chart** under `helm/dbhub-operator/`, and ginkgo `test/e2e`.

## Why it matters

`DBHubInstance` is the **operator-native alternative** to the hand-written
`k8s/postgres-mcp` / `k8s/neo4j-mcp` Deployments: instead of editing YAML to add
a database, you create a `Database` CR and the operator regenerates config and
rolls the MCP server automatically. The hand-written `k8s/dbhub/` manifests are
the non-operator (static) way to run the same `bytebase/dbhub` image.
