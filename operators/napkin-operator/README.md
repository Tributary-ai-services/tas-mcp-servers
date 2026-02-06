# Napkin Visual Operator

Kubernetes operator that watches `NapkinVisual` custom resources and reconciles them through the full async lifecycle: submit to Napkin AI, poll for completion, download generated files, and store them permanently in MinIO.

## CRD: NapkinVisual

| Field | Description |
|-------|-------------|
| **Group** | `napkin.tas.ai` |
| **Version** | `v1` |
| **Kind** | `NapkinVisual` |
| **Short name** | `nv` |

## State Machine

```
Pending -> Submitted -> Processing -> Downloading -> Uploading -> Completed
  |           |            |              |              |
  +--------+--+------------+--------------+--------------+---> Failed
```

## Example CR

```yaml
apiVersion: napkin.tas.ai/v1
kind: NapkinVisual
metadata:
  name: architecture-diagram
  namespace: tas-mcp-servers
spec:
  content: "Steps to deploy a microservice: 1. Build Docker image 2. Push to registry 3. Apply K8s manifests 4. Verify health"
  format: svg
  style:
    colorMode: light
  apiKeySecretRef:
    name: napkin-api-secret
    key: NAPKIN_API_KEY
  storage:
    bucket: napkin-visuals
```

## Commands

```bash
make build        # Build binary
make test         # Run tests
make install      # Install CRD
make deploy       # Deploy operator
make docker-build # Build Docker image
```

## Ports

| Port | Purpose |
|------|---------|
| 8088 | Metrics endpoint |
| 8089 | Health probes |
