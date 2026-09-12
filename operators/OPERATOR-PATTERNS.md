# TAS MCP Operator Patterns

`tas-mcp-servers/operators/` contains three Kubernetes operators, all built on
`sigs.k8s.io/controller-runtime`. They sit at different layers and represent
three points on the operator-complexity spectrum. This is the overview; each
operator has its own `docs/PATTERN.md` with the detail.

## The three operators

| Operator | CRD(s) / Group | What it manages | Owns K8s child objects? | Style |
|---|---|---|---|---|
| **napkin-operator** | `NapkinVisual` (`napkin.tas.ai/v1`) | An external task: call Napkin AI, store output in MinIO | No — manages *external* state (API + object store) | Hand-rolled, finalizer for external cleanup |
| **datasource-operator** | `DatasourceConnection` (`datasource.tas.ai/v1`) | Health of an external datasource, probed *through* the MCP bridges | No — only reads Secrets, writes status | Hand-rolled, phase machine |
| **dbhub-operator** | `Database` + `DBHubInstance` (`dbhub.tas.io/v1alpha1`) | Deploys DBHub MCP servers + the DB connections they expose | **Yes** — owns Deployment/Service/ConfigMap/Secret | Full kubebuilder/operator-sdk + webhooks + Helm |

## The shared "house pattern"

Every TAS operator follows the same conventions:

- **Spec/Status CRD** with `+kubebuilder:subresource:status`, printer columns,
  enum/default/min/max validation markers, `observedGeneration`.
- **Level-triggered reconcile** driven by a `phase` field (most use an explicit
  `Pending → … → Completed/Connected/Failed` state machine), persisting progress
  to `.status` via `r.Status().Update()` and requeuing with `RequeueAfter`.
- **Secret-referenced credentials** (`*SecretRef`) rather than inline secrets.
- **Multi-tenancy fields** (`tenantId`/`spaceId`) where relevant.
- **Manager bootstrap** in `cmd/`: scheme registration, metrics + health probes,
  optional leader election.
- **TAS layout split:** operator source under `operators/<name>/`, runtime deploy
  manifests under `k8s/<name>/`; deployments default to **BestEffort QoS** per the
  cluster resource policy.

## Three layers of database access (answers "do neo4j/postgres MCP use operators?")

```
STATIC (no operator)        k8s/neo4j-mcp, k8s/postgres-mcp  = Deployment + ConfigMap + Secret + Service
                            (tas-mcp-bridge image, MCP_COMMAND/MCP_ARGS)

CONTROL PLANE (operator)    datasource-operator              = health-tests connections THROUGH those bridges
                            (DatasourceConnection CR)

OPERATOR-NATIVE (operator)  dbhub-operator                   = generates config + Deployment/Service for a
                            (Database + DBHubInstance CRs)     DBHub MCP server; add a Database CR and it
                                                               regenerates config and rolls the pods
```

So the neo4j/postgres MCP **servers** themselves are plain ConfigMap-driven
Deployments (no operator). Operators exist *around* them: `datasource-operator`
verifies connection health, and `dbhub-operator` is the operator-native way to
run database MCP servers declaratively.

## Complexity ladder (use as templates)

- **Simplest** — `datasource-operator`: status-only, no children, no finalizer.
  Good template for a "watch a CR and reflect external state" operator.
- **External-resource lifecycle** — `napkin-operator`: adds a **finalizer** to
  clean up non-Kubernetes resources (MinIO objects) on delete.
- **Full deployment operator** — `dbhub-operator`: `Owns()` child objects with
  owner references + GC, cross-resource `Watches()`, config-hash rollouts,
  admission webhooks (cert-manager), the kubebuilder `config/` tree, and a Helm
  chart. Use when the CR must materialize real workloads.

## Per-operator detail

- [napkin-operator/docs/PATTERN.md](napkin-operator/docs/PATTERN.md)
- [datasource-operator/docs/PATTERN.md](datasource-operator/docs/PATTERN.md)
- [dbhub-operator/docs/PATTERN.md](dbhub-operator/docs/PATTERN.md) *(source on branch `feature/neo4j-datasource`)*
