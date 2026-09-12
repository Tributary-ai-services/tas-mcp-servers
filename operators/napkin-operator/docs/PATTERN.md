# Napkin Operator Pattern

This document describes the Kubernetes operator pattern implemented by the
Napkin operator. It is the canonical controller-runtime operator in TAS
(contrast with `tas-mcp`, which is a Kustomize-deployed service, not an operator).

## What it is

A **kubebuilder-style Kubernetes operator** that turns a piece of text into
rendered diagrams via the external **Napkin AI** API, then persists the results
to **MinIO** — all driven declaratively through a `NapkinVisual` custom resource.

**Stack:** Go 1.22 · `sigs.k8s.io/controller-runtime` v0.17.2 ·
`k8s.io/{api,apimachinery,client-go}` v0.29.3 · OpenTelemetry tracing · MinIO SDK.

## Repository layout (TAS convention)

The operator follows the kubebuilder project layout, but TAS splits
**build-time code** from **runtime manifests** across two top-level dirs:

```
operators/napkin-operator/            # operator source (kubebuilder layout)
├── api/v1/
│   ├── groupversion_info.go          # GroupVersion {napkin.tas.ai, v1} + SchemeBuilder
│   ├── napkinvisual_types.go         # Spec/Status Go types + kubebuilder markers
│   └── zz_generated.deepcopy.go      # controller-gen output (DeepCopy methods)
├── pkg/controllers/
│   └── napkinvisual_controller.go    # Reconciler (the control loop)
├── pkg/napkin/  pkg/minio/           # external API + object-store clients
├── cmd/operator/main.go              # manager bootstrap / entrypoint
├── deployments/kubernetes/crds/      # CRD + RBAC (shipped with the operator)
├── Dockerfile · Makefile · go.mod

k8s/napkin-operator/                  # runtime deploy overlay (kustomize)
├── deployment.yaml · service.yaml · configmap.yaml · rbac.yaml · kustomization.yaml
```

This `operators/<name>` (source) + `k8s/<name>` (deploy) split is the repeatable
TAS pattern. `Makefile: deploy` installs the CRD then
`kubectl apply -k ../../k8s/napkin-operator/`.

## 1. The API type (the contract)

`api/v1/napkinvisual_types.go` defines the CRD as Go structs with **Spec/Status
separation**, the core of the declarative model:

- **`NapkinVisualSpec`** = *desired state*: `content`, `format` (svg/png/ppt),
  `style`, `variations`, `tenantId`, `apiKeySecretRef` (points at a Secret rather
  than inlining the key), and `storage` (MinIO bucket/prefix).
- **`NapkinVisualStatus`** = *observed state*: a `phase`, `conditions[]`,
  `napkinRequestId`, `generatedFiles[]` (with both the ephemeral Napkin URL and
  the permanent MinIO key/URL), `retryCount`, `observedGeneration`, timestamps.

The kubebuilder markers drive both codegen and API-server validation:

```go
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status                       // status is a separate subresource
//+kubebuilder:printcolumn:name="Phase",...JSONPath=".status.phase"
//+kubebuilder:resource:shortName=nv
```

Field-level markers (`+kubebuilder:validation:Enum`, `MinLength`, `default=svg`,
etc.) become the OpenAPI v3 schema. `groupversion_info.go` registers the group
**`napkin.tas.ai/v1`** into the scheme via `SchemeBuilder`.

## 2. The CRD manifest

`deployments/kubernetes/crds/napkinvisual_crd.yaml` is the cluster-facing
`CustomResourceDefinition`:

- Group/Version/Kind: `napkin.tas.ai/v1`, `NapkinVisual` (plural `napkinvisuals`,
  short name `nv`, categories `napkin`/`tas`).
- `scope: Namespaced`, `subresources.status: {}` enabled.
- Full OpenAPI v3 validation schema mirroring the Go markers.
- `additionalPrinterColumns` for `kubectl get nv` (Format, Phase, Files, Age).

## 3. The reconciler (the control loop)

`pkg/controllers/napkinvisual_controller.go` is the heart of the pattern.
`NapkinVisualReconciler` embeds `client.Client` and holds the `Scheme`, an OTel
`tracer`, the Napkin base URL, and a MinIO client.

It implements a **level-triggered, phase-based state machine**. Each `Reconcile`
invocation does a small slice of work, persists progress to `.status`, and requeues:

```
Pending ──submit──▶ Submitted/Processing ──poll──▶ Downloading ──fetch+store──▶ Completed
   │                       │                            │
   └────────────── Failed (retryCount<3 ⇒ requeue 5m) ◀┘
```

Reconcile flow (`napkinvisual_controller.go:54`):

1. **Fetch** the CR; `IsNotFound` → return cleanly (it was deleted).
2. **Finalizer handling** (`:77`) — adds `napkinvisual.napkin.tas.ai/finalizer`
   on create; on deletion runs `cleanupVisual()` (deletes the MinIO objects)
   before removing the finalizer. This is the key reason the operator owns
   *external* state.
3. **Initialize status** to `Pending` with a `Ready=False` condition on first
   sight, then `Requeue`.
4. **Dispatch on `status.phase`** to a per-phase handler:
   - `reconcilePending` → reads the API key from the referenced Secret, calls
     `napkin.Submit()`, records `napkinRequestId`, → `Submitted` (requeue 5s).
   - `reconcilePolling` → polls Napkin; on `completed` captures file metadata →
     `Downloading`; on `failed` → `Failed`.
   - `reconcileDownloading` → downloads each file from the (30-min-expiry)
     Napkin URL, uploads to MinIO at `prefix/tenantId/name/index.format`, writes
     back `MinioKey`/`MinioUrl`, sets `Completed` + `Ready=True` +
     `observedGeneration`.
   - `Failed` → auto-retry after 5m while `retryCount < 3`.

Requeue strategy is the standard controller-runtime idiom:
`ctrl.Result{RequeueAfter: ...}` for polling/backoff, `{Requeue: true}` for
immediate phase transitions, `{}` for terminal states. Status is always written
via `r.Status().Update()` (the status subresource).

Registration (`napkinvisual_controller.go:391`):

```go
func (r *NapkinVisualReconciler) SetupWithManager(mgr ctrl.Manager) error {
    r.tracer = otel.Tracer("napkinvisual-controller")
    return ctrl.NewControllerManagedBy(mgr).
        For(&napkinv1.NapkinVisual{}).
        Complete(r)
}
```

It only `For()`s the CR — there are **no `Owns()` clauses and no owner
references**, because the managed resources live *outside* Kubernetes (the Napkin
API + MinIO objects), not as child K8s objects. That is what makes
finalizer-based cleanup essential here rather than relying on garbage collection.

## 4. Manager bootstrap

`cmd/operator/main.go` is the standard manager entrypoint:

- Registers `clientgoscheme` + `napkinv1` into the runtime scheme (`init()`).
- Builds the MinIO client (endpoint/keys via flags or `MINIO_*` env, plus an
  optional `MINIO_PUBLIC_URL` for external download links).
- `ctrl.NewManager(ctrl.GetConfigOrDie(), …)` with **metrics on `:8088`**,
  **health/ready probes on `:8089`**, and **configurable leader election**
  (`LeaderElectionID: napkin-operator-leader-election`,
  `LeaderElectionReleaseOnCancel: true`).
- Wires the `NapkinVisualReconciler` via `SetupWithManager`, adds `healthz`/
  `readyz` ping checks, then `mgr.Start(ctrl.SetupSignalHandler())`.

## 5. RBAC (least privilege)

RBAC is generated from `//+kubebuilder:rbac` markers above `Reconcile` and
shipped as `deployments/kubernetes/crds/rbac.yaml`:

- **ClusterRole** `napkin-operator-manager`: full verbs on `napkinvisuals`,
  `get/update/patch` on `napkinvisuals/status`, `update` on
  `napkinvisuals/finalizers`, `get/list/watch` on `secrets`, `create/patch` on
  `events`.
- **ServiceAccount** `napkin-operator` in `tas-mcp-servers` + **ClusterRoleBinding**
  tying them together.

The Secret read permission is exactly what `getAPIKey()` needs; nothing broader.

## 6. Deployment

`k8s/napkin-operator/deployment.yaml`: single replica,
`serviceAccountName: napkin-operator`, image
`registry-api.tas.scharber.com/napkin-operator:1.0.0`, args `--leader-elect=false`,
env from the `napkin-operator-config` ConfigMap, liveness/readiness against `:8089`.
Per the TAS resource policy, **requests/limits are intentionally omitted →
BestEffort QoS** ("until profiled via Prometheus/Grafana"), with a comment to
that effect.

## 7. Code generation & build

`Makefile` targets: `build`, `test`, `docker-build/push`, `install`/`uninstall`
(apply/delete the CRD), `deploy`/`undeploy` (kustomize), `run` (run locally
against your kubeconfig), `ci` (fmt+vet+test+build). `zz_generated.deepcopy.go` is
produced by `controller-gen` (installed at `~/go/bin/controller-gen`, run as
`controller-gen object paths="./api/v1/"`).

## Pattern summary

The reusable template: CRD with Spec/Status + status subresource → a Reconciler
that runs a phase-based state machine, persists progress to `.status`, requeues
with backoff, and uses a **finalizer to clean up external (non-K8s) resources** →
manager with metrics/health/leader-election → marker-generated RBAC → split
`operators/<name>` source + `k8s/<name>` deploy dirs.

## Things to know (current deviations)

- **The CRD YAML is hand-maintained**, not generated by a Makefile target (there
  is no `manifests`/`generate` target here). It currently mirrors the Go markers,
  but the two can drift — regenerate deliberately.
- **`reconcileUploading` is effectively dead code** — download and upload are
  combined in `reconcileDownloading`; the `Uploading` phase is never entered.
- **Conditions are replaced wholesale** (`Status.Conditions = []…{…}`) rather than
  merged via `meta.SetStatusCondition`, so only one condition is ever present at a
  time. Fine today, but it discards history if multiple condition types are added.
- A fresh Napkin client is constructed per reconcile (cheap, stateless — no concern).
