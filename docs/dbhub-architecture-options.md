# DBHub Architecture Options for Multi-User Support

> **Last Updated:** January 2026
> **Status:** Decision Required

## Key Finding

**DBHub configures database connections at startup time only.** There is no runtime API to dynamically add/remove database sources. Connections are defined via:

1. `--dsn` flag (single database)
2. `--config` flag pointing to TOML file (multiple databases)
3. Environment variables

These are **mutually exclusive** - you use either `--dsn` OR `--config`, not both.

---

## DBHub Invocation Options

### Single Database Mode

```bash
dbhub --transport http --port 8080 --dsn "postgres://user:pass@host:5432/db"
```

### Multi-Database Mode (TOML)

```bash
dbhub --transport http --port 8080 --config /path/to/dbhub.toml
```

**dbhub.toml example:**
```toml
[[sources]]
id = "production"
dsn = "postgres://user:pass@prod-host:5432/db"

[[sources]]
id = "staging"
dsn = "postgres://user:pass@staging-host:5432/db"

[[tools]]
name = "execute_sql"
source = "production"
readonly = true
max_rows = 1000

[[tools]]
name = "execute_sql"
source = "staging"
readonly = false
```

### Instance ID for Multiple Instances

```bash
# Instance 1
dbhub --transport http --port 8080 --id "prod" --dsn "postgres://..."

# Instance 2
dbhub --transport http --port 8081 --id "staging" --dsn "postgres://..."
```

The `--id` flag suffixes tool names (e.g., `execute_sql_prod`, `execute_sql_staging`).

---

## Architecture Options

### Option A: Pre-Configured Sources (Static TOML)

**Description:** Define all known databases in TOML at deployment time. Users select from available sources when calling MCP tools.

```
┌─────────────────────────────────────────────────────────┐
│                      DBHub Pod                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │              dbhub.toml                          │   │
│  │  - tas-postgres-shared                          │   │
│  │  - analytics-db                                 │   │
│  │  - staging-db                                   │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
          ↑
    MCP Clients select source by ID
```

**Pros:**
- Simple deployment
- Single pod serves all users
- Low resource usage

**Cons:**
- New databases require redeployment
- All users see all databases (no isolation)
- Credentials stored in ConfigMap/Secret

**Best for:** Internal tools with known, stable database list

---

### Option B: Per-User/Tenant DBHub Instances

**Description:** Each user/tenant gets their own DBHub pod with their specific DSN.

```
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│  DBHub (user-1)  │  │  DBHub (user-2)  │  │  DBHub (user-3)  │
│  --dsn=user1-db  │  │  --dsn=user2-db  │  │  --dsn=user3-db  │
└──────────────────┘  └──────────────────┘  └──────────────────┘
         ↑                    ↑                    ↑
      User 1               User 2               User 3
```

**Pros:**
- Full isolation between users
- Each user controls their own credentials
- No shared state

**Cons:**
- High resource usage (1 pod per user)
- Complex orchestration needed
- Pod lifecycle management required

**Best for:** Multi-tenant SaaS with strict isolation requirements

---

### Option C: Dynamic TOML Generation (Sidecar Pattern)

**Description:** Store database configs in a service (e.g., Aether backend). A sidecar or controller generates TOML and restarts DBHub when configs change.

```
┌─────────────────────────────────────────────────────────┐
│                      DBHub Pod                          │
│  ┌─────────────┐    ┌─────────────────────────────┐    │
│  │  Sidecar    │───▶│         DBHub               │    │
│  │  (config    │    │  --config /config/dbhub.toml│    │
│  │   generator)│    └─────────────────────────────┘    │
│  └─────────────┘                                       │
│        ↑                                               │
└────────│───────────────────────────────────────────────┘
         │
    ┌────┴────┐
    │ Aether  │  (database registry)
    │ Backend │
    └─────────┘
```

**Pros:**
- Databases can be added via UI
- Centralized credential management
- Single pod, multiple databases

**Cons:**
- Requires config reload mechanism
- DBHub restart on config change (brief downtime)
- Additional component to maintain

**Best for:** Platform with UI-managed database connections

---

### Option D: On-Demand Ephemeral Instances

**Description:** A gateway/controller spawns short-lived DBHub instances on-demand when users need database access.

```
┌─────────────────────────────────────────────────────────┐
│                   MCP Gateway                           │
│  ┌─────────────────────────────────────────────────┐   │
│  │  Receives MCP request with DSN                   │   │
│  │  Spawns ephemeral DBHub container                │   │
│  │  Proxies request → DBHub → response              │   │
│  │  Terminates container after idle timeout         │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
                        │
         ┌──────────────┼──────────────┐
         ↓              ↓              ↓
    ┌─────────┐   ┌─────────┐   ┌─────────┐
    │ DBHub   │   │ DBHub   │   │ DBHub   │
    │ (temp)  │   │ (temp)  │   │ (temp)  │
    └─────────┘   └─────────┘   └─────────┘
```

**Pros:**
- Maximum flexibility
- DSN passed per-request
- No stored credentials
- Auto-scaling

**Cons:**
- Complex gateway implementation
- Container startup latency
- Resource management complexity

**Best for:** Serverless/FaaS style workloads

---

### Option E: Alternative MCP Server (FreePeak DB MCP)

**Description:** Use [FreePeak/db-mcp-server](https://github.com/FreePeak/db-mcp-server) which has more dynamic connection management.

**Pros:**
- Built for multi-database scenarios
- May have runtime configuration API

**Cons:**
- Less mature than DBHub
- Need to evaluate feature parity
- Different configuration approach

**Best for:** If DBHub doesn't meet requirements

---

## Recommendation for TAS

### Short-Term (Phase 1): Option A - Static TOML

Deploy DBHub with pre-configured TAS infrastructure databases:
- `tas-postgres-shared`
- Future: `tas-mysql`, `analytics-db`, etc.

This provides immediate value with minimal complexity.

### Medium-Term (Phase 2): Option C - Dynamic TOML

Build integration with Aether backend:
1. UI for managing database connections
2. Store encrypted credentials in Aether/Keycloak
3. Sidecar generates TOML from Aether API
4. ConfigMap reloader triggers DBHub restart

### Long-Term (Phase 3): Evaluate Option D or E

Based on usage patterns, consider:
- On-demand instances for true multi-tenancy
- Alternative MCP servers with better dynamic support

---

## Decision Required

Which architecture option should we implement?

| Option | Complexity | Isolation | Flexibility | Recommended Phase |
|--------|:----------:|:---------:|:-----------:|:-----------------:|
| A: Static TOML | Low | None | Low | Phase 1 |
| B: Per-User Pods | High | Full | Medium | - |
| C: Dynamic TOML | Medium | Partial | Medium | Phase 2 |
| D: Ephemeral | High | Full | High | Phase 3 |
| E: Alternative | Medium | Varies | High | Evaluate |

---

## References

- [DBHub GitHub](https://github.com/bytebase/dbhub)
- [DBHub Configuration](https://dbhub.ai/config/toml)
- [FreePeak DB MCP Server](https://github.com/FreePeak/db-mcp-server)
