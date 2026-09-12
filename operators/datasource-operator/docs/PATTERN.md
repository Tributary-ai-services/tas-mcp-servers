# Datasource Operator Pattern

A lightweight, hand-rolled controller-runtime operator whose job is **connection
health**, not deployment. It models an external datasource as a custom resource
and continuously verifies it is reachable **through the existing MCP bridges** —
it does not create any Deployments/ConfigMaps. See
`operators/OPERATOR-PATTERNS.md` for how it relates to `napkin-operator` and
`dbhub-operator`.

## Custom resource: `DatasourceConnection` (`datasource.tas.ai/v1`)

`api/v1/datasourceconnection_types.go`.

- **Spec (desired):** `type` (enum `neo4j;postgres;minio;kafka;grafana`), `host`,
  `port`, `database`, `protocol`, `sslMode`, `credentialsSecretRef`
  (Secret + username/password keys), multi-tenancy (`tenantId`/`spaceId`/
  `ownerUserId`), `bridgeEndpoint` (optional override of the MCP bridge URL), and
  `healthCheckInterval` (default 300s).
- **Status (observed):** `phase` (`Pending;Testing;Connected;Failed`),
  `lastTestedAt`, `lastTestResult`, `latencyMs`, `message`, `observedGeneration`.
- Markers: `+kubebuilder:subresource:status` + printer columns
  (Type/Host/Phase/Latency/Age).

## Reconcile: a four-state machine

`pkg/controllers/datasourceconnection_controller.go` dispatches on
`status.phase`:

```
"" / Pending ──validate secret──▶ Testing ──probe via bridge──▶ Connected
       │                              │                              │
       └──────────── Failed ◀─────────┴───── (re-test every interval) ┘
```

- **`reconcilePending`** — verifies the credentials Secret exists and contains
  the username/password keys; on success → `Testing` (immediate requeue), else
  `Failed` with a message and backoff.
- **`reconcileTesting`** — looks up a per-type test-tool definition
  (`bridge.TestToolDefs[type]`) and a bridge endpoint
  (`spec.bridgeEndpoint` or `bridge.DefaultBridgeEndpoints[type]`), reads the
  credentials, injects them into the tool params, and calls
  `BridgeClient.CallTool(ctx, bridgeURL, tool, params)`. Success → `Connected`
  with `latencyMs`; failure → `Failed`.
- **`reconcileConnected` / `reconcileFailed`** — record results and requeue after
  `healthCheckInterval` for continuous re-verification / backoff.

The key architectural choice: **the operator does not talk to databases
directly** — it reuses the deployed MCP bridge servers (neo4j-mcp, postgres-mcp,
minio-mcp, kafka-mcp, grafana-mcp) as the connectivity-test transport via their
tool API. That keeps driver/protocol logic in the bridges and makes the operator
a thin control-plane over connection state.

## Structure

Hand-rolled minimal layout (like `napkin-operator`, not the full kubebuilder
`config/` tree): `api/v1/` (types, groupversion, deepcopy), `pkg/controllers/`,
`pkg/bridge/` (the MCP bridge HTTP client + per-type `TestToolDefs` /
`DefaultBridgeEndpoints`), `cmd/operator/main.go`, and
`deployments/kubernetes/` (CRD, RBAC, deployment, service). No child-object
ownership, no finalizer, no admission webhook — it only reads Secrets and writes
its own `.status`.
