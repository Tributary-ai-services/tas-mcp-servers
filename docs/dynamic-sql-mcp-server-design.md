# Dynamic SQL MCP Server Design

> **Last Updated:** January 2026
> **Status:** Proposed
> **Problem:** Existing MCP database servers (DBHub, etc.) configure DSN at startup, not per-request

## Problem Statement

Users have multiple database environments:
- Local Docker containers
- K3s clusters
- Dev, Test, Staging, Production environments
- Different projects with different databases

Existing MCP servers require pre-configuring all databases at deployment time. This doesn't work when:
1. Users bring their own databases
2. Environments are dynamic
3. Multiple users have different database access

**Required:** Pass DSN per-request as a tool parameter.

---

## Proposed Solution: Custom MCP Server

Build a lightweight MCP server that accepts connection parameters as tool arguments.

### Tool Signatures

```python
@mcp.tool()
async def execute_sql(
    dsn: str,           # Connection string: postgres://user:pass@host:5432/db
    query: str,         # SQL query to execute
    params: list = [],  # Query parameters (for prepared statements)
    readonly: bool = True,  # Safety: default to read-only
    max_rows: int = 1000,   # Limit result set
    timeout: int = 30       # Query timeout in seconds
) -> dict:
    """Execute SQL query against any supported database."""
    pass

@mcp.tool()
async def list_tables(
    dsn: str,
    schema: str = "public"
) -> list:
    """List tables in database."""
    pass

@mcp.tool()
async def describe_table(
    dsn: str,
    table: str,
    schema: str = "public"
) -> dict:
    """Get table schema (columns, types, constraints)."""
    pass

@mcp.tool()
async def explain_query(
    dsn: str,
    query: str
) -> dict:
    """Get query execution plan."""
    pass
```

### Usage Example (Claude API)

```python
response = client.beta.messages.create(
    model="claude-sonnet-4-20250514",
    mcp_servers=[{
        "type": "url",
        "url": "https://sql-mcp.tas.scharber.com/sse",
        "name": "dynamic-sql",
        "authorization_token": "USER_TOKEN"
    }],
    messages=[{
        "role": "user",
        "content": "List all users from my staging database"
    }],
    # The LLM will call execute_sql with user-provided DSN
)
```

### MCP Tool Call

```json
{
  "tool": "execute_sql",
  "parameters": {
    "dsn": "postgres://myuser:mypass@staging-db.example.com:5432/myapp",
    "query": "SELECT * FROM users LIMIT 10",
    "readonly": true
  }
}
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Claude / AI Agent                         │
│  mcp_servers: [{ url: "https://sql-mcp.tas.scharber.com" }] │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│              Dynamic SQL MCP Server                          │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  Tools:                                              │    │
│  │  - execute_sql(dsn, query, ...)                     │    │
│  │  - list_tables(dsn, schema)                         │    │
│  │  - describe_table(dsn, table)                       │    │
│  │  - explain_query(dsn, query)                        │    │
│  └─────────────────────────────────────────────────────┘    │
│                              │                               │
│                    Per-request connection                    │
│                              │                               │
└──────────────────────────────│───────────────────────────────┘
                               │
         ┌─────────────────────┼─────────────────────┐
         ▼                     ▼                     ▼
    ┌─────────┐          ┌─────────┐          ┌─────────┐
    │ User's  │          │ User's  │          │ User's  │
    │ Dev DB  │          │ Staging │          │ Prod DB │
    └─────────┘          └─────────┘          └─────────┘
```

---

## Implementation

### Technology Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Python 3.11+ | MCP SDK maturity, SQLAlchemy support |
| MCP SDK | `mcp` (PyPI) | Official Anthropic SDK |
| DB Library | SQLAlchemy 2.0 | Multi-database support |
| Transport | HTTP/SSE | Remote access, stateless |
| Container | Python slim | Lightweight |

### Supported Databases

Via SQLAlchemy:
- PostgreSQL (`postgresql://`)
- MySQL (`mysql://`)
- MariaDB (`mariadb://`)
- SQLite (`sqlite:///`)
- SQL Server (`mssql://`)

### Core Implementation

```python
from mcp import MCPServer
from sqlalchemy import create_engine, text
from sqlalchemy.exc import SQLAlchemyError
import asyncio

mcp = MCPServer("dynamic-sql")

@mcp.tool()
async def execute_sql(
    dsn: str,
    query: str,
    params: list = [],
    readonly: bool = True,
    max_rows: int = 1000,
    timeout: int = 30
) -> dict:
    """
    Execute SQL query against any supported database.

    Args:
        dsn: Database connection string (e.g., postgres://user:pass@host:5432/db)
        query: SQL query to execute
        params: Query parameters for prepared statements
        readonly: If True, only SELECT queries allowed
        max_rows: Maximum rows to return
        timeout: Query timeout in seconds

    Returns:
        dict with 'columns', 'rows', 'row_count'
    """
    # Validate readonly mode
    if readonly:
        query_upper = query.strip().upper()
        if not query_upper.startswith(('SELECT', 'SHOW', 'DESCRIBE', 'EXPLAIN')):
            return {"error": "Only SELECT queries allowed in readonly mode"}

    try:
        # Create engine with timeout
        engine = create_engine(
            dsn,
            connect_args={"connect_timeout": 10},
            pool_pre_ping=True
        )

        with engine.connect() as conn:
            # Set statement timeout
            if 'postgresql' in dsn:
                conn.execute(text(f"SET statement_timeout = {timeout * 1000}"))

            # Execute query
            result = conn.execute(text(query), params)

            if result.returns_rows:
                columns = list(result.keys())
                rows = [dict(row._mapping) for row in result.fetchmany(max_rows)]
                return {
                    "columns": columns,
                    "rows": rows,
                    "row_count": len(rows),
                    "truncated": result.rowcount > max_rows if result.rowcount else False
                }
            else:
                return {
                    "affected_rows": result.rowcount,
                    "success": True
                }

    except SQLAlchemyError as e:
        return {"error": str(e)}
    finally:
        engine.dispose()

@mcp.tool()
async def list_tables(dsn: str, schema: str = "public") -> dict:
    """List all tables in the specified schema."""
    try:
        engine = create_engine(dsn)
        from sqlalchemy import inspect
        inspector = inspect(engine)
        tables = inspector.get_table_names(schema=schema)
        engine.dispose()
        return {"schema": schema, "tables": tables}
    except SQLAlchemyError as e:
        return {"error": str(e)}

@mcp.tool()
async def describe_table(dsn: str, table: str, schema: str = "public") -> dict:
    """Get detailed schema information for a table."""
    try:
        engine = create_engine(dsn)
        from sqlalchemy import inspect
        inspector = inspect(engine)

        columns = []
        for col in inspector.get_columns(table, schema=schema):
            columns.append({
                "name": col["name"],
                "type": str(col["type"]),
                "nullable": col["nullable"],
                "default": str(col.get("default", "")) if col.get("default") else None
            })

        pk = inspector.get_pk_constraint(table, schema=schema)
        fks = inspector.get_foreign_keys(table, schema=schema)
        indexes = inspector.get_indexes(table, schema=schema)

        engine.dispose()
        return {
            "table": table,
            "schema": schema,
            "columns": columns,
            "primary_key": pk,
            "foreign_keys": fks,
            "indexes": indexes
        }
    except SQLAlchemyError as e:
        return {"error": str(e)}

if __name__ == "__main__":
    mcp.run(transport="streamable-http", host="0.0.0.0", port=8080)
```

---

## Security Considerations

### 1. DSN in Requests

**Risk:** DSN contains credentials, transmitted per-request.

**Mitigations:**
- HTTPS required (TLS encryption)
- Authorization token validates user
- Audit logging of connections (without passwords)
- Rate limiting per user

### 2. Query Injection

**Risk:** Malicious queries.

**Mitigations:**
- `readonly=True` by default (only SELECT)
- Parameterized queries supported
- Query timeout limits
- Row count limits

### 3. Network Access

**Risk:** MCP server can reach any database.

**Mitigations:**
- Network policies limit egress
- Allowlist specific CIDR ranges
- DNS filtering if needed

### 4. Credential Storage

**Best Practice:** Users should use short-lived credentials or connection through a bastion/proxy rather than embedding long-lived passwords in DSN.

---

## Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dynamic-sql-mcp
  namespace: tas-mcp-servers
spec:
  replicas: 2
  selector:
    matchLabels:
      app: dynamic-sql-mcp
  template:
    metadata:
      labels:
        app: dynamic-sql-mcp
    spec:
      containers:
      - name: dynamic-sql-mcp
        image: tas-registry/dynamic-sql-mcp:latest
        ports:
        - containerPort: 8080
        env:
        - name: LOG_LEVEL
          value: "INFO"
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: 1000m
            memory: 1Gi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
```

---

## Comparison: DBHub vs Dynamic SQL MCP

| Feature | DBHub | Dynamic SQL MCP |
|---------|-------|-----------------|
| DSN Configuration | Startup only | Per-request |
| Multi-database | Via TOML | Any database |
| Pre-configuration | Required | None |
| User isolation | None | Per-request |
| Credential storage | Server-side | Client-side |
| Complexity | Low | Medium |
| Existing solution | Yes | Build custom |

---

## Implementation Plan

### Phase 1: Core Server (Week 1-2)
1. Create Python MCP server project
2. Implement `execute_sql` tool
3. Implement `list_tables`, `describe_table`
4. Add basic auth/token validation
5. Dockerize

### Phase 2: Deployment (Week 2-3)
1. Create K8s manifests
2. Deploy to tas-mcp-servers namespace
3. Configure ingress with TLS
4. Test with Claude API

### Phase 3: Enhancements (Week 3-4)
1. Add connection pooling per DSN
2. Audit logging
3. Rate limiting
4. Prometheus metrics

---

## Decision

**Recommendation:** Build the Dynamic SQL MCP Server.

This solves the core problem: users can connect to any database from any environment without server-side pre-configuration.

**Alternative:** If build effort is too high, consider:
1. Contributing per-request DSN feature to DBHub
2. Using DBHub with per-user instances (Option B from previous doc)

---

## References

- [MCP Python SDK](https://github.com/modelcontextprotocol/python-sdk)
- [SQLAlchemy 2.0](https://docs.sqlalchemy.org/en/20/)
- [MCP Specification](https://modelcontextprotocol.io/specification/)
