# FederatedMCPServer Operator

A Kubernetes operator that keeps a **tas-mcp federation gateway's** server
registry in sync with declarative `FederatedMCPServer` custom resources. Each CR
declares one downstream MCP server; the operator registers it on the running
gateway (and unregisters it on delete), so the gateway federates exactly the set
of servers described in the cluster.

This is the **Slice B** server-source for reduce-at-source: once a server is
registered, its tool-call results flow through `Manager.InvokeServer` and are
reduced + compliance-scanned at source (see `tas-mcp` `internal/federation` and
`internal/reduction`).

## How it works

```
FederatedMCPServer CR ──watch──► operator ──REST──► tas-mcp gateway
                                   (reconcile)        POST   /api/v1/federation/servers
                                                      DELETE /api/v1/federation/servers/{id}
                                                        └─► Manager.RegisterServer / UnregisterServer
```

The operator talks to the gateway over its existing federation REST API rather
than sharing the Manager in-process, which keeps the gateway binary free of
controller-runtime and mirrors the other TAS operators
(`napkin-operator`, `datasource-operator`).

- **Create / update** → build the registration payload from the CR spec
  (resolving the auth `Secret` if referenced) → `POST /servers`. A duplicate-id
  conflict triggers an unregister-then-register so spec changes take effect.
- **Delete** → a finalizer runs `DELETE /servers/{id}` before the CR is removed,
  so the gateway never keeps a stale server.
- **Drift healing** → the gateway's registry is **in-memory**, so a gateway
  restart silently drops every registration. On each reconcile the operator
  verifies the server is actually present (`GET /servers/{id}`) instead of
  trusting its own status, and re-registers if it's gone. Every healthy CR is
  requeued on `--resync-interval` (default 2m), so the gateway self-heals within
  one interval of a restart even with no CR changes.
- **Status** → `.status.phase` (`Pending`/`Registered`/`Failed`),
  `.status.registered`, `.status.registeredID`, and a `Registered` condition.

## CRD

`FederatedMCPServer` (`mcp.tas.ai/v1`, short name `fms`). Key spec fields:

| Field | Notes |
|-------|-------|
| `serverID` | Stable federation id; defaults to the CR name |
| `displayName` | Required |
| `endpoint` | Required — URL the gateway calls |
| `protocol` | `http` (default) / `grpc` / `sse` / `stdio` |
| `auth.type` | `none` (default) / `api_key` / `oauth2` / `jwt` / `basic` / `bearer` |
| `auth.secretRef` | Secret whose keys become the auth config map (values never inlined) |
| `capabilities`, `tags`, `metadata` | Passed through to the registration |

See `config/samples/` for examples.

## Configuration (operator flags / env)

| Flag | Env | Default |
|------|-----|---------|
| `--gateway-url` | `TAS_MCP_GATEWAY_URL` | `http://prod-tas-mcp-http.tas-mcp-prod.svc.cluster.local:8082` |
| `--gateway-timeout` | — | `10s` |
| `--resync-interval` | — | `2m` (drift re-check cadence) |
| `--metrics-bind-address` | — | `:8088` |
| `--health-probe-bind-address` | — | `:8089` |
| `--leader-elect` | — | `false` |

## Develop

```bash
make generate manifests   # regenerate deepcopy + CRD/RBAC (needs controller-gen)
make build vet            # compile + vet
make docker-build docker-push IMG=registry-api.tas.scharber.com/federated-mcp-operator:0.1.0
make install deploy       # apply CRD, RBAC, Deployment
make samples              # apply the example CRs
```

## Status: skeleton

This is a working skeleton. Implemented: register/re-register/unregister with a
finalizer, auth-Secret resolution, and **drift healing** (periodic gateway-state
verification + re-register after a gateway restart), with `httptest` coverage of
the gateway REST contract.

Before production:
- Add controller-level tests (envtest) for the reconcile paths (finalizer,
  re-register on spec change, drift re-register, secret resolution).
- Decide leader-election + replica strategy and whether one operator manages
  multiple gateways.
