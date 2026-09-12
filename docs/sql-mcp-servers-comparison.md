# SQL Database MCP Servers - Comparison Matrices

> **Last Updated:** January 2026
> **Purpose:** Feature and database support comparison for SQL MCP server selection

## Table of Contents

- [Database Support Matrix](#database-support-matrix)
- [Core Capabilities Matrix](#core-capabilities-matrix)
- [Advanced Features Matrix](#advanced-features-matrix)
- [Real-Time & CDC Matrix](#real-time--cdc-matrix)
- [Security & Access Control Matrix](#security--access-control-matrix)
- [Deployment & Integration Matrix](#deployment--integration-matrix)
- [Project Health Matrix](#project-health-matrix)
- [Summary Scorecard](#summary-scorecard)
- [Recommended Stack for TAS](#recommended-stack-for-tas)

---

## Database Support Matrix

| MCP Server | PostgreSQL | MySQL | MariaDB | SQL Server | SQLite | Oracle | Cloud SQL | AlloyDB | Spanner | Other |
|------------|:----------:|:-----:|:-------:|:----------:|:------:|:------:|:---------:|:-------:|:-------:|-------|
| **DBHub** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | - |
| **MCP Alchemy** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | CrateDB, Vertica, any SQLAlchemy |
| **Google MCP Toolbox** | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ | ✅ | ✅ | ✅ | Bigtable, Neo4j, Dgraph |
| **mcp-database-server** | ✅ | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | - |
| **Postgres MCP Pro** | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | - |
| **postgresql-mcp-server** | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | - |
| **mcp-postgres-full-access** | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | - |
| **mysql_mcp_server** | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | - |
| **mcp-server-mysql** | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | - |
| **mcp-sqlite** | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | - |
| **sqlite-explorer-fastmcp** | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | - |
| **mssql_mcp_server** | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | Azure SQL, LocalDB |

### Database Coverage Summary

| Database | # of Servers Supporting |
|----------|:-----------------------:|
| PostgreSQL | 8 |
| MySQL | 6 |
| SQL Server | 5 |
| SQLite | 5 |
| MariaDB | 3 |
| Oracle | 1 |
| Cloud SQL | 1 |
| AlloyDB | 1 |
| Spanner | 1 |

---

## Core Capabilities Matrix

| MCP Server | Read Queries | Write Queries | Schema Inspection | CRUD Operations | Transaction Support | Parameterized Queries |
|------------|:------------:|:-------------:|:-----------------:|:---------------:|:-------------------:|:---------------------:|
| **DBHub** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **MCP Alchemy** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Google MCP Toolbox** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **mcp-database-server** | ✅ | ✅ | ✅ | ✅ | ⚠️ | ✅ |
| **Postgres MCP Pro** | ✅ | ✅* | ✅ | ✅* | ✅ | ✅ |
| **postgresql-mcp-server** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **mcp-postgres-full-access** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **mysql_mcp_server** | ✅ | ✅ | ✅ | ✅ | ⚠️ | ✅ |
| **mcp-server-mysql** | ✅ | ❌ | ✅ | ❌ | ❌ | ✅ |
| **mcp-sqlite** | ✅ | ✅ | ✅ | ✅ | ⚠️ | ✅ |
| **sqlite-explorer-fastmcp** | ✅ | ❌ | ✅ | ❌ | ❌ | ✅ |
| **mssql_mcp_server** | ✅ | ✅ | ✅ | ✅ | ⚠️ | ✅ |

**Legend:**
- ✅ Full support
- ✅* Configurable - can be restricted to read-only
- ⚠️ Limited or implicit support
- ❌ Not supported

---

## Advanced Features Matrix

| MCP Server | EXPLAIN Plans | Index Analysis | Performance Monitoring | Health Checks | Query Optimization | Data Import/Export |
|------------|:-------------:|:--------------:|:----------------------:|:-------------:|:------------------:|:------------------:|
| **DBHub** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **MCP Alchemy** | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| **Google MCP Toolbox** | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ |
| **mcp-database-server** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Postgres MCP Pro** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| **postgresql-mcp-server** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **mcp-postgres-full-access** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **mysql_mcp_server** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **mcp-server-mysql** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **mcp-sqlite** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **sqlite-explorer-fastmcp** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **mssql_mcp_server** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |

### Advanced Features Summary

| Feature | Best Server(s) |
|---------|---------------|
| EXPLAIN Plans | Postgres MCP Pro, postgresql-mcp-server |
| Index Analysis | Postgres MCP Pro, postgresql-mcp-server |
| Performance Monitoring | Postgres MCP Pro, postgresql-mcp-server, Google MCP Toolbox |
| Health Checks | Postgres MCP Pro, postgresql-mcp-server |
| Query Optimization | Postgres MCP Pro, postgresql-mcp-server |
| Data Import/Export | postgresql-mcp-server, MCP Alchemy |

---

## Real-Time & CDC Matrix

| MCP Server | CDC Support | Change Streaming | WAL/Binlog Access | Event Notifications | Real-Time Monitoring |
|------------|:-----------:|:----------------:|:-----------------:|:-------------------:|:--------------------:|
| **DBHub** | ❌ | ❌ | ❌ | ❌ | ❌ |
| **MCP Alchemy** | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Google MCP Toolbox** | ❌ | ❌ | ❌ | ❌ | ⚠️ |
| **mcp-database-server** | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Postgres MCP Pro** | ❌ | ❌ | ❌ | ❌ | ✅ |
| **postgresql-mcp-server** | ❌ | ❌ | ❌ | ❌ | ✅ |
| **mcp-postgres-full-access** | ❌ | ❌ | ❌ | ❌ | ❌ |
| **mysql_mcp_server** | ❌ | ❌ | ❌ | ❌ | ❌ |
| **mcp-server-mysql** | ❌ | ❌ | ❌ | ❌ | ❌ |
| **mcp-sqlite** | ❌ | ❌ | N/A | ❌ | ❌ |
| **sqlite-explorer-fastmcp** | ❌ | ❌ | N/A | ❌ | ❌ |
| **mssql_mcp_server** | ❌ | ❌ | ❌ | ❌ | ❌ |

**Key Finding:** No MCP database server currently supports CDC (Change Data Capture). All servers use query-based interaction patterns.

For CDC requirements, see [CDC Analysis Document](./sql-mcp-servers-cdc-analysis.md) for alternative approaches using Debezium + Kafka.

---

## Security & Access Control Matrix

| MCP Server | Read-Only Mode | Connection Pooling | SSL/TLS | SSH Tunneling | Auth Integration | Row Limits | Query Timeout |
|------------|:--------------:|:------------------:|:-------:|:-------------:|:----------------:|:----------:|:-------------:|
| **DBHub** | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | ✅ |
| **MCP Alchemy** | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ |
| **Google MCP Toolbox** | ❌ | ✅ | ✅ | ❌ | ✅ (OAuth2/OIDC) | ❌ | ❌ |
| **mcp-database-server** | ❌ | ❌ | ⚠️ | ❌ | ❌ | ❌ | ❌ |
| **Postgres MCP Pro** | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ |
| **postgresql-mcp-server** | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| **mcp-postgres-full-access** | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| **mysql_mcp_server** | ❌ | ❌ | ⚠️ | ❌ | ❌ | ❌ | ❌ |
| **mcp-server-mysql** | ✅ | ❌ | ⚠️ | ❌ | ❌ | ❌ | ❌ |
| **mcp-sqlite** | ❌ | N/A | N/A | N/A | N/A | ✅ | ❌ |
| **sqlite-explorer-fastmcp** | ✅ | N/A | N/A | N/A | N/A | ❌ | ❌ |
| **mssql_mcp_server** | ❌ | ❌ | ✅ | ❌ | ✅ (Windows/Azure AD) | ❌ | ❌ |

### Security Features Summary

| Feature | Best Server(s) |
|---------|---------------|
| Read-Only Mode | DBHub, Postgres MCP Pro, mcp-server-mysql, sqlite-explorer-fastmcp |
| Connection Pooling | DBHub, MCP Alchemy, Google MCP Toolbox, Postgres MCP Pro, postgresql-mcp-server |
| SSL/TLS | All major servers |
| SSH Tunneling | DBHub (only) |
| Auth Integration | Google MCP Toolbox (OAuth2/OIDC), mssql_mcp_server (Windows/Azure AD) |
| Row Limits | DBHub, MCP Alchemy, mcp-sqlite |
| Query Timeout | DBHub, Postgres MCP Pro |

---

## Deployment & Integration Matrix

| MCP Server | Docker Image | npm/npx | pip/uvx | Stdio Transport | HTTP/SSE Transport | Web UI | OpenTelemetry |
|------------|:------------:|:-------:|:-------:|:---------------:|:------------------:|:------:|:-------------:|
| **DBHub** | ✅ | ✅ | ❌ | ✅ | ✅ | ✅ | ❌ |
| **MCP Alchemy** | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| **Google MCP Toolbox** | ✅ | ✅ | ❌ | ✅ | ✅ | ❌ | ✅ |
| **mcp-database-server** | ❌ | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ |
| **Postgres MCP Pro** | ✅ | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ |
| **postgresql-mcp-server** | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ |
| **mcp-postgres-full-access** | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| **mysql_mcp_server** | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| **mcp-server-mysql** | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| **mcp-sqlite** | ❌ | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ |
| **sqlite-explorer-fastmcp** | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| **mssql_mcp_server** | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |

### Deployment Summary

| Deployment Method | Servers Supporting |
|-------------------|-------------------|
| Docker Image | DBHub, Google MCP Toolbox, Postgres MCP Pro, postgresql-mcp-server, mssql_mcp_server |
| npm/npx | DBHub, Google MCP Toolbox, mcp-database-server, postgresql-mcp-server, mcp-sqlite |
| pip/uvx | MCP Alchemy, Postgres MCP Pro, mcp-postgres-full-access, mysql_mcp_server, mcp-server-mysql, sqlite-explorer-fastmcp, mssql_mcp_server |
| HTTP/SSE Transport | DBHub, Google MCP Toolbox, Postgres MCP Pro |
| Web UI | DBHub (only) |
| OpenTelemetry | Google MCP Toolbox (only) |

---

## Project Health Matrix

| MCP Server | License | Stars | Last Update | Language | Maturity |
|------------|---------|------:|-------------|----------|----------|
| **DBHub** | MIT | 2,000+ | Active | TypeScript | Production |
| **MCP Alchemy** | MPL-2.0 | 386 | Active | Python | Production |
| **Google MCP Toolbox** | Apache-2.0 | High | Active | Go/TS | Beta |
| **mcp-database-server** | - | Low | Active | TypeScript | Stable |
| **Postgres MCP Pro** | MIT | High | Active | Python | Production |
| **postgresql-mcp-server** | AGPL-3.0 | Medium | Active | TypeScript | Production |
| **mcp-postgres-full-access** | - | Low | Active | Python | Stable |
| **mysql_mcp_server** | MIT | 1,100+ | Active | Python | Production |
| **mcp-server-mysql** | - | Low | Active | Python | Stable |
| **mcp-sqlite** | MIT | 81 | Active | JavaScript | Stable |
| **sqlite-explorer-fastmcp** | - | Low | Active | Python | Stable |
| **mssql_mcp_server** | MIT | 297 | Active | Python | Production |

### License Compatibility

| License | Servers | Commercial Use | Modification | Distribution |
|---------|---------|:--------------:|:------------:|:------------:|
| MIT | DBHub, Postgres MCP Pro, mysql_mcp_server, mcp-sqlite, mssql_mcp_server | ✅ | ✅ | ✅ |
| Apache-2.0 | Google MCP Toolbox | ✅ | ✅ | ✅ |
| MPL-2.0 | MCP Alchemy | ✅ | ✅ | ✅ (with conditions) |
| AGPL-3.0 | postgresql-mcp-server | ✅ | ✅ | ✅ (copyleft) |

---

## Summary Scorecard

| MCP Server | DB Coverage | Features | Security | Deployment | Overall Rating |
|------------|:-----------:|:--------:|:--------:|:----------:|:--------------:|
| **DBHub** | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | **Best Multi-DB** |
| **Google MCP Toolbox** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | **Best Cloud** |
| **MCP Alchemy** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | **Best Flexibility** |
| **Postgres MCP Pro** | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | **Best PostgreSQL** |
| **postgresql-mcp-server** | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | **Best PG Tools** |
| **mysql_mcp_server** | ⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | **Best MySQL** |
| **mssql_mcp_server** | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | **Best SQL Server** |
| **mcp-sqlite** | ⭐ | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ | **Best SQLite** |

---

## Recommended Stack for TAS

Based on the comparison matrices, here is the recommended layered approach for TAS MCP Servers:

### Primary Layer

| Server | Purpose | Priority |
|--------|---------|:--------:|
| **DBHub** | Universal SQL access (PostgreSQL, MySQL, MariaDB, SQL Server, SQLite) | P0 |

**Rationale:**
- Covers 5 major SQL databases with single deployment
- Docker-ready (fits existing K8s pattern)
- MIT licensed (no restrictions)
- Most active community (2k+ stars)
- Built-in safety features (read-only mode, row limits, query timeouts)

### PostgreSQL Enhanced Layer

| Server | Purpose | Priority |
|--------|---------|:--------:|
| **Postgres MCP Pro** | Advanced optimization, health monitoring, production safety | P1 |

**Rationale:**
- Complements DBHub with advanced PostgreSQL features
- Industrial-strength index tuning algorithms
- Health analysis and EXPLAIN plans
- Configurable read-only mode for production `tas-postgres-shared`
- Docker-ready, MIT licensed

### Cloud Integration Layer (Optional)

| Server | Purpose | Priority |
|--------|---------|:--------:|
| **Google MCP Toolbox** | Cloud SQL, AlloyDB, Spanner integration | P2 |

**Rationale:**
- Only option for Google Cloud database services
- Built-in OAuth2/OIDC and OpenTelemetry
- Apache-2.0 license
- Note: Currently in beta - evaluate for production readiness

### Coverage Analysis

With the recommended stack:

| Database | Coverage |
|----------|:--------:|
| PostgreSQL | ✅✅ (DBHub + Postgres MCP Pro) |
| MySQL | ✅ (DBHub) |
| MariaDB | ✅ (DBHub) |
| SQL Server | ✅ (DBHub) |
| SQLite | ✅ (DBHub) |
| Cloud SQL | ✅ (Google MCP Toolbox) |
| AlloyDB | ✅ (Google MCP Toolbox) |
| Spanner | ✅ (Google MCP Toolbox) |
