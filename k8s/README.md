# TAS MCP Servers - Kubernetes Deployments

This directory contains Kubernetes deployment manifests for MCP servers in the TAS platform.

## Directory Structure

```
k8s/
├── kustomization.yaml    # Parent kustomization for all MCP servers
├── README.md             # This file
└── crawl4ai/             # Crawl4AI web scraping server
    ├── kustomization.yaml
    ├── deployment.yaml
    ├── service.yaml
    ├── configmap.yaml
    └── ingress.yaml
```

## Deployment

### Deploy All MCP Servers

```bash
kubectl apply -k k8s/
```

### Deploy Individual Server

```bash
kubectl apply -k k8s/crawl4ai/
```

## Available Servers

### Crawl4AI

Web scraping service using headless browsers for intelligent content extraction.

| Property | Value |
|----------|-------|
| Image | `unclecode/crawl4ai:latest` |
| Port | 11235 |
| Namespace | `tas-mcp-servers` |
| Internal URL | `http://crawl4ai.tas-mcp-servers.svc.cluster.local:11235` |
| External URL | `https://crawl4ai.tas.scharber.com` |

**Endpoints:**
- `/health` - Health check
- `/crawl` - Crawl URLs and extract content
- `/playground` - Interactive testing UI

**Requirements:**
- Shared memory: 3GB minimum for headless browsers
- Memory: 2-4GB RAM
- CPU: 500m-2000m

## Adding New MCP Servers

1. Create a new directory under `k8s/` with the server name
2. Add the following manifests:
   - `deployment.yaml` - Pod specification
   - `service.yaml` - ClusterIP service
   - `configmap.yaml` - Configuration
   - `ingress.yaml` - External access (optional)
   - `kustomization.yaml` - Kustomize config
3. Update the parent `kustomization.yaml` to include the new server
4. Document the server in this README

## Integration with TAS Infrastructure

These servers are deployed to the `tas-shared` namespace and integrate with:
- **TAS MCP Federation** - Unified access to MCP capabilities
- **cert-manager** - Automatic TLS certificates via Let's Encrypt
- **NGINX Ingress** - External access with SSL termination
- **Prometheus** - Metrics collection (if exposed)
