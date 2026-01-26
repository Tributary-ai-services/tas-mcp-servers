# DBHub Implementation Plan

> **Last Updated:** January 2026
> **Status:** Planning
> **Priority:** P0 - Primary SQL MCP Server
> **Namespace:** `tas-mcp-servers`
> **Directory:** `/home/jscharber/eng/TAS/tas-mcp-servers/k8s/dbhub/`

## Overview

Deploy [DBHub](https://github.com/bytebase/dbhub) as the primary multi-database MCP server within the `tas-mcp-servers` namespace, following the same deployment pattern as Crawl4AI.

### Repository Structure

```
tas-mcp-servers/
├── k8s/
│   ├── namespace.yaml              # tas-mcp-servers namespace
│   ├── kustomization.yaml          # Parent kustomization
│   ├── crawl4ai/                   # Existing - web scraping
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── configmap.yaml
│   │   ├── ingress.yaml
│   │   └── kustomization.yaml
│   └── dbhub/                      # NEW - multi-database SQL
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── configmap.yaml
│       ├── ingress.yaml
│       └── kustomization.yaml
│       # NOTE: secret created externally via UI or kubectl
└── docs/
    ├── sql-mcp-servers-comparison.md
    ├── sql-mcp-servers-cdc-analysis.md
    └── dbhub-implementation-plan.md
```

---

## Technical Specifications

| Property | Value |
|----------|-------|
| **Image** | `bytebase/dbhub:latest` |
| **Port** | 8080 |
| **Transport** | HTTP |
| **Namespace** | `tas-mcp-servers` |
| **Internal URL** | `http://dbhub.tas-mcp-servers.svc.cluster.local:8080` |
| **External URL** | `https://dbhub.tas.scharber.com` |

### Supported Databases

- PostgreSQL
- MySQL
- MariaDB
- SQL Server
- SQLite

### MCP Tools

| Tool | Description |
|------|-------------|
| `execute_sql` | Execute SQL queries with transaction support |
| `search_objects` | Search database objects (tables, columns, schemas) |

---

## Implementation Phases

### Phase 1: Basic Deployment

**Goal:** Deploy DBHub connected to `tas-postgres-shared`

**Tasks:**
1. Create `k8s/dbhub/` directory
2. Create Kubernetes manifests (deployment, service, configmap, secret, ingress)
3. Update parent `k8s/kustomization.yaml` to include dbhub
4. Deploy and verify connectivity

### Phase 2: Multi-Database Configuration

**Goal:** Add connections to additional TAS databases

**Tasks:**
1. Create TOML configuration for multiple databases
2. Add read-only connection for production safety
3. Test connections to all configured databases

### Phase 3: Security & Monitoring

**Goal:** Production hardening

**Tasks:**
1. Configure network policies
2. Enable Prometheus metrics scraping
3. Add to Alloy log collection
4. Create Grafana dashboard

---

## Kubernetes Manifests

### deployment.yaml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dbhub
  namespace: tas-mcp-servers
  labels:
    app: dbhub
    component: mcp-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dbhub
  template:
    metadata:
      labels:
        app: dbhub
        component: mcp-server
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
    spec:
      containers:
        - name: dbhub
          image: bytebase/dbhub:latest
          args:
            - "--transport"
            - "http"
            - "--port"
            - "8080"
            - "--dsn"
            - "$(DATABASE_DSN)"
          ports:
            - containerPort: 8080
              name: http
          env:
            - name: DATABASE_DSN
              valueFrom:
                secretKeyRef:
                  name: dbhub-secrets
                  key: DATABASE_DSN
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 512Mi
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
      restartPolicy: Always
```

### service.yaml

```yaml
apiVersion: v1
kind: Service
metadata:
  name: dbhub
  namespace: tas-mcp-servers
  labels:
    app: dbhub
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
      protocol: TCP
      name: http
  selector:
    app: dbhub
```

### Secret (Created Externally)

The `dbhub-secrets` secret must be created externally via UI or kubectl - **not stored in git**.

**Via kubectl:**
```bash
kubectl create secret generic dbhub-secrets \
  --namespace=tas-mcp-servers \
  --from-literal=DATABASE_DSN="postgres://user:pass@host:5432/db?sslmode=disable"
```

**Via UI:**
- Navigate to Secrets management
- Create secret named `dbhub-secrets` in `tas-mcp-servers` namespace
- Add key `DATABASE_DSN` with the connection string value

**DSN Format Examples:**
```
# PostgreSQL
postgres://user:password@host:5432/dbname?sslmode=disable

# MySQL
mysql://user:password@host:3306/dbname

# SQL Server
sqlserver://user:password@host:1433?database=dbname
```

### configmap.yaml

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dbhub-config
  namespace: tas-mcp-servers
data:
  LOG_LEVEL: "INFO"
```

### ingress.yaml

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: dbhub-ingress
  namespace: tas-mcp-servers
  annotations:
    cert-manager.io/cluster-issuer: "tas-ca-issuer"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "120"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "120"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - dbhub.tas.scharber.com
      secretName: dbhub-tls
  rules:
    - host: dbhub.tas.scharber.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: dbhub
                port:
                  number: 8080
```

### kustomization.yaml

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: tas-mcp-servers

resources:
  - deployment.yaml
  - service.yaml
  - configmap.yaml
  - ingress.yaml

# NOTE: Secret 'dbhub-secrets' must be created externally via UI or kubectl

commonLabels:
  app.kubernetes.io/name: dbhub
  app.kubernetes.io/component: mcp-server
  app.kubernetes.io/part-of: tas-mcp-servers
```

---

## Deployment Commands

```bash
# 1. Create secret first (via UI or kubectl)
kubectl create secret generic dbhub-secrets \
  --namespace=tas-mcp-servers \
  --from-literal=DATABASE_DSN="postgres://tasuser:taspassword@tas-postgres-shared.tas-shared.svc.cluster.local:5432/tas_shared?sslmode=disable"

# 2. Deploy DBHub
kubectl apply -k /home/jscharber/eng/TAS/tas-mcp-servers/k8s/dbhub/

# 3. Verify deployment
kubectl get pods -n tas-mcp-servers -l app=dbhub
kubectl logs -n tas-mcp-servers -l app=dbhub

# 4. Test connectivity
kubectl port-forward -n tas-mcp-servers svc/dbhub 8080:8080
curl http://localhost:8080/health
```

---

## Parent Kustomization Update

Update `k8s/kustomization.yaml` to include dbhub:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - namespace.yaml
  - crawl4ai
  - dbhub          # ADD THIS LINE
```

---

## Success Criteria

| Metric | Target |
|--------|--------|
| Pod Status | Running |
| Health Check | 200 OK |
| PostgreSQL Connection | Successful |
| MCP Tool Execution | Working |

---

## References

- [DBHub GitHub](https://github.com/bytebase/dbhub)
- [SQL MCP Servers Comparison](./sql-mcp-servers-comparison.md)
- [Crawl4AI Example](../k8s/crawl4ai/)
