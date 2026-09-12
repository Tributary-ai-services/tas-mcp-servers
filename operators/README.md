# TAS MCP Operators

This directory contains the Kubernetes operators for the TAS MCP platform. All
are built on `sigs.k8s.io/controller-runtime` and follow the shared TAS operator
conventions (Spec/Status CRD, phase-driven level-triggered reconcile,
Secret-referenced credentials, `operators/<name>` source + `k8s/<name>` deploy
split, BestEffort QoS by default).

**Start here:** [OPERATOR-PATTERNS.md](OPERATOR-PATTERNS.md) — overview, the
shared "house pattern", the three-layer database-access model, and a
complexity ladder for choosing a template.

## Operators

| Operator | CRD(s) / Group | Role | Detail |
|---|---|---|---|
| **napkin-operator** | `NapkinVisual` (`napkin.tas.ai/v1`) | External task: Napkin AI → MinIO. Finalizer cleans up external objects. | [docs/PATTERN.md](napkin-operator/docs/PATTERN.md) |
| **datasource-operator** | `DatasourceConnection` (`datasource.tas.ai/v1`) | Connection-health prober via the MCP bridges. Status-only, no children. | [docs/PATTERN.md](datasource-operator/docs/PATTERN.md) |
| **dbhub-operator** | `Database` + `DBHubInstance` (`dbhub.tas.io/v1alpha1`) | Full deployment operator: generates config and owns Deployment/Service/ConfigMap/Secret for DBHub MCP servers. | [docs/PATTERN.md](dbhub-operator/docs/PATTERN.md) |

> **Note:** the `dbhub-operator` source currently lives on branch
> `feature/neo4j-datasource` (the `main` checkout has only build artifacts under
> `dbhub-operator/`). Read it with
> `git show feature/neo4j-datasource:operators/dbhub-operator/<path>`.
