# SQL Database MCP Servers - Research Summary

> **Last Updated:** January 2026
> **Purpose:** Comprehensive research on available MCP servers for PostgreSQL and SQL database integration

## Overview

This document provides research findings on Model Context Protocol (MCP) servers available for SQL database integration. The research covers multi-database solutions, database-specific servers, and recommendations for the TAS MCP Servers platform.

## Table of Contents

- [Multi-Database Universal Solutions](#multi-database-universal-solutions)
- [PostgreSQL-Specific Servers](#postgresql-specific-servers)
- [MySQL-Specific Servers](#mysql-specific-servers)
- [SQLite-Specific Servers](#sqlite-specific-servers)
- [MS SQL Server](#ms-sql-server)
- [Recommended Options](#recommended-options)
- [Sources](#sources)

---

## Multi-Database Universal Solutions

These servers support multiple database types through a single interface:

### DBHub (Bytebase)

- **Repository:** https://github.com/bytebase/dbhub
- **Databases:** PostgreSQL, MySQL, MariaDB, SQL Server, SQLite
- **License:** MIT
- **Stars:** 2,000+
- **Language:** TypeScript (98.9%)

**Key Features:**
- Zero-dependency architecture optimized for token efficiency
- Multi-database simultaneous connections via TOML configuration
- Read-only mode with row limiting and query timeouts for safety
- SSH tunneling and SSL/TLS encryption support
- Built-in web workbench interface for query execution and visualization

**MCP Tools:**
1. `execute_sql` - Run queries with transaction support and safety guardrails
2. `search_objects` - Explore schemas, tables, columns, indexes, and procedures
3. Custom Tools - Define parameterized reusable operations in dbhub.toml

**Installation:**
```bash
# Docker
docker run --rm --init --name dbhub --publish 8080:8080 bytebase/dbhub \
  --transport http --port 8080 \
  --dsn "postgres://user:password@localhost:5432/dbname?sslmode=disable"

# NPM/CLI
npx @bytebase/dbhub@latest --transport http --port 8080 \
  --dsn "postgres://user:password@localhost:5432/dbname?sslmode=disable"
```

---

### MCP Alchemy

- **Repository:** https://github.com/runekaagaard/mcp-alchemy
- **Databases:** PostgreSQL, MySQL, MariaDB, SQLite, Oracle, MS SQL Server, CrateDB, Vertica, + any SQLAlchemy-compatible
- **License:** MPL-2.0
- **Stars:** 386
- **Language:** Python

**Key Features:**
- Database schema exploration and understanding
- SQL query writing and validation support
- Table relationship visualization
- Large dataset analysis and reporting
- Integration with claude-local-files for handling extensive result sets

**MCP Tools:**
- `all_table_names` - Returns database table listing
- `filter_table_names` - Searches tables by substring matching
- `schema_definitions` - Comprehensive table schemas
- `execute_query` - Runs SQL with vertical result formatting

**Configuration:**
```bash
# Environment Variables
DB_URL=postgresql://user:pass@host:5432/db
CLAUDE_LOCAL_FILES_PATH=/path/to/results
EXECUTE_QUERY_MAX_CHARS=4000
```

---

### Google MCP Toolbox for Databases

- **Repository:** https://github.com/googleapis/genai-toolbox
- **Documentation:** https://googleapis.github.io/genai-toolbox/
- **Databases:** AlloyDB, Cloud SQL (PostgreSQL/MySQL/SQL Server), Spanner, Bigtable, Neo4j, Dgraph
- **License:** Apache-2.0
- **Status:** Beta

**Key Features:**
- Query in Plain English - natural language database interaction
- Automate Database Management - AI-driven schema management
- Simplified development with reduced boilerplate code
- Enhanced security through OAuth2 and OIDC
- End-to-end observability with OpenTelemetry integration
- Connection pooling and authentication management

**PostgreSQL-Specific Tools:**
- `postgres-sql` - Execute SQL queries as prepared statements
- `postgres-execute-sql` - Run parameterized SQL statements
- `postgres-list-tables`
- `postgres-list-active-queries`
- `postgres-list-available-extensions`
- `postgres-list-installed-extensions`
- `postgres-list-views`
- `postgres-list-schemas`
- `postgres-database-overview`

**Installation:**
```bash
# Homebrew
brew install mcp-toolbox

# Docker
docker pull us-central1-docker.pkg.dev/[project]/genai-toolbox

# NPM
npm install @toolbox-sdk/server
```

---

### mcp-database-server (ExecuteAutomation)

- **Repository:** https://github.com/executeautomation/mcp-database-server
- **Databases:** SQLite, SQL Server, PostgreSQL, MySQL
- **Language:** TypeScript

**Installation:**
```bash
npm install -g @executeautomation/database-server
```

---

## PostgreSQL-Specific Servers

### Postgres MCP Pro (Crystal DBA)

- **Repository:** https://github.com/crystaldba/postgres-mcp
- **License:** MIT
- **Language:** Python

**Key Features:**
- Database Health Analysis - index integrity, connection utilization, buffer cache, vacuum operations
- Index Tuning - industrial-strength algorithms (based on MS SQL Server Database Tuning Advisor)
- Query Optimization - EXPLAIN plans and hypothetical index simulation
- Schema Intelligence - context-aware SQL generation
- Safe SQL Execution - configurable read-only mode for production

**MCP Tools (8 Primary):**
1. `list_schemas` - Enumerate database schemas
2. `list_objects` - Display tables, views, sequences, extensions
3. `get_object_details` - Retrieve column info, constraints, indexes
4. `execute_sql` - Execute SQL with read-only limitations when restricted
5. `explain_query` - Generate execution plans with hypothetical indexes
6. `get_top_queries` - Identify slowest queries via pg_stat_statements
7. `analyze_workload_indexes` - Recommend indexes for resource-intensive queries
8. `analyze_db_health` - Comprehensive health assessment

**Access Modes:**
- **Unrestricted:** Full read/write capabilities (development)
- **Restricted:** Read-only transactions with execution time constraints (production)

**Installation:**
```bash
# Docker
docker pull crystaldba/postgres-mcp

# Python (pipx)
pipx install postgres-mcp

# Python (uv)
uv pip install postgres-mcp
```

**Optional Extensions:**
```sql
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
CREATE EXTENSION IF NOT EXISTS hypopg;
```

---

### postgresql-mcp-server (HenkDz)

- **Repository:** https://github.com/HenkDz/postgresql-mcp-server
- **License:** AGPL-3.0
- **Language:** TypeScript

**Architecture:**
- Original: 46 individual tools
- Current: 17 intelligent tools (8 meta-tools + 5 specialized + 4 new)

**Tool Categories:**

*Meta-Tools (Consolidated):*
- Schema management (tables, columns, ENUMs, constraints)
- User & permission administration
- Query performance analysis
- Index optimization
- Stored function management
- Database triggers
- Constraint handling
- Row-level security policies

*Enhancement Tools (New):*
- Query execution with SELECT operations
- Data mutation (INSERT/UPDATE/DELETE/UPSERT)
- Arbitrary SQL execution with transaction support
- Comment management across database objects

*Specialized Tools:*
- Database performance analysis
- Debugging utilities
- Data import/export (JSON/CSV)
- Cross-database data transfers
- Real-time monitoring

**Installation:**
```bash
# npm
npm install -g @henkey/postgres-mcp-server

# Docker (recommended for production)
docker pull henkey/postgres-mcp-server
```

---

### Other PostgreSQL Servers

| Server | Repository | Key Differentiator |
|--------|------------|-------------------|
| **pg-mcp-server** | https://github.com/stuzero/pg-mcp-server | Enhanced AI agent capabilities |
| **mcp-postgres-full-access** | https://github.com/syahiidkamil/mcp-postgres-full-access | Full read-write (vs. official read-only) |
| **postgres-mcp-server** | https://github.com/ahmedmustahid/postgres-mcp-server | HTTP + Stdio transports |
| **Official MCP PostgreSQL** | Archived at modelcontextprotocol/servers-archived | Read-only, schema inspection |

---

## MySQL-Specific Servers

### mysql_mcp_server (Design Computer)

- **Repository:** https://github.com/designcomputer/mysql_mcp_server
- **License:** MIT
- **Stars:** 1,100+
- **Language:** Python (93.2%)
- **Latest Release:** v0.2.2 (April 2025)

**Features:**
- List available MySQL tables as resources
- Read table contents
- Execute SQL queries with error handling
- Secure access via environment variables
- Comprehensive logging capabilities
- Dedicated SECURITY.md guide

**Configuration:**
```bash
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=username
MYSQL_PASSWORD=password
MYSQL_DATABASE=dbname
```

**Installation:**
```bash
pip install mysql-mcp-server

# Or via Smithery
npx -y @smithery/cli install mysql-mcp-server --client claude
```

---

### mcp-server-mysql (Ben Borla)

- **Repository:** https://github.com/benborla/mcp-server-mysql
- **Features:** Read-only access, schema inspection, safe queries

---

## SQLite-Specific Servers

### mcp-sqlite (eQuill Labs)

- **Repository:** https://github.com/jparkerweb/mcp-sqlite
- **License:** MIT
- **Stars:** 81
- **Language:** JavaScript (100%)

**MCP Tools:**

*Database Information:*
- `db_info` - Returns connected database metadata
- `list_tables` - Enumerates all tables
- `get_table_schema` - Detailed column information

*Data Manipulation:*
- `create_record` - Inserts new records
- `read_records` - Queries with filtering, limit, offset
- `update_records` - Modifies matching records
- `delete_records` - Removes based on filter criteria

*Query:*
- `query` - Executes arbitrary SQL with parameter binding

**Configuration (Cursor):**
```json
{
  "mcpServers": {
    "MCP SQLite Server": {
      "command": "npx",
      "args": ["-y", "mcp-sqlite", "<database-path>"]
    }
  }
}
```

---

### Other SQLite Servers

| Server | Repository | Key Differentiator |
|--------|------------|-------------------|
| **sqlite-explorer-fastmcp** | https://github.com/hannesrudolph/sqlite-explorer-fastmcp-mcp-server | Read-only, FastMCP framework |
| **sqlitecloud-mcp-server** | https://github.com/sqlitecloud/sqlitecloud-mcp-server | SQLite Cloud integration |

---

## MS SQL Server

### mssql_mcp_server (Richard Han)

- **Repository:** https://github.com/RichardHan/mssql_mcp_server
- **License:** MIT
- **Stars:** 297
- **Language:** Python (93.2%)
- **Latest Release:** v0.1.0 (June 2025)

**Features:**
- Database exploration - list tables and schema information
- Query execution - SELECT, INSERT, UPDATE, DELETE
- Multiple authentication: SQL Server, Windows integrated, Azure AD
- Database support: Local SQL Server, LocalDB, Azure SQL
- Custom port configuration

**Configuration:**
```bash
# Required
MSSQL_SERVER=hostname
MSSQL_DATABASE=dbname

# SQL Authentication
MSSQL_USER=username
MSSQL_PASSWORD=password

# Windows Authentication
MSSQL_WINDOWS_AUTH=true

# Azure SQL
# Server: your-server.database.windows.net

# Optional
MSSQL_PORT=1433
MSSQL_ENCRYPT=true
```

**Installation:**
```bash
pip install microsoft_sql_server_mcp
```

---

## Recommended Options

### Tier 1 - Primary Choices

1. **DBHub** - Best multi-database solution
   - Covers PostgreSQL, MySQL, MariaDB, SQL Server, SQLite
   - Zero dependencies, MIT licensed
   - Docker-ready, TypeScript codebase
   - Active development (2k+ stars)

2. **Postgres MCP Pro** - Best PostgreSQL-specific
   - Industrial-strength index tuning
   - Health analysis and query optimization
   - Configurable read-only mode
   - Docker-ready, MIT licensed

### Tier 2 - Specialized

3. **Google MCP Toolbox** - Best for cloud databases
   - Google Cloud SQL, AlloyDB, Spanner
   - Built-in OAuth2/OIDC and OpenTelemetry
   - Apache-2.0 license
   - Note: Currently in beta

4. **mysql_mcp_server** - Best MySQL-only
   - 1.1k stars, active community
   - Comprehensive security documentation

5. **mcp-sqlite** - Best SQLite
   - Full CRUD support
   - Simple npm package

---

## Sources

- [mcpservers.org Database Category](https://mcpservers.org/category/database)
- [Official MCP Servers Repository](https://github.com/modelcontextprotocol/servers)
- [DBHub - Bytebase](https://github.com/bytebase/dbhub)
- [Postgres MCP Pro](https://github.com/crystaldba/postgres-mcp)
- [MCP Alchemy](https://github.com/runekaagaard/mcp-alchemy)
- [Google MCP Toolbox](https://github.com/googleapis/genai-toolbox)
- [MySQL MCP Server](https://github.com/designcomputer/mysql_mcp_server)
- [MCP SQLite](https://github.com/jparkerweb/mcp-sqlite)
- [MS SQL MCP Server](https://github.com/RichardHan/mssql_mcp_server)
- [PostgreSQL MCP Server (HenkDz)](https://github.com/HenkDz/postgresql-mcp-server)
