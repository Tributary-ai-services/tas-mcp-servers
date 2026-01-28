# CDC (Change Data Capture) Support Analysis

> **Last Updated:** January 2026
> **Purpose:** Analysis of CDC capabilities in SQL MCP servers

## Summary

**None of the surveyed MCP database servers currently support CDC (Change Data Capture) natively.**

The existing MCP database servers are designed for **query-based interaction** (request/response pattern), not real-time change streaming. CDC requires:
- Database log monitoring (WAL, binlog, transaction logs)
- Persistent connections for change streaming
- Event-based push notifications

These patterns differ significantly from the MCP protocol's current design.

---

## CDC Support Matrix

| MCP Server | CDC Support | Notes |
|------------|:-----------:|-------|
| **DBHub** | ❌ | Query-based only |
| **MCP Alchemy** | ❌ | Query-based only |
| **Google MCP Toolbox** | ❌ | Query-based only |
| **Postgres MCP Pro** | ❌ | Query-based, includes `pg_stat_statements` but no streaming |
| **postgresql-mcp-server** | ❌ | Query-based, real-time monitoring but no CDC |
| **mysql_mcp_server** | ❌ | Query-based only |
| **mcp-sqlite** | ❌ | Query-based only |
| **mssql_mcp_server** | ❌ | Query-based only |

---

## Why CDC is Missing from MCP Servers

### 1. Protocol Design
MCP (Model Context Protocol) is designed for:
- **Request/Response**: Client asks, server responds
- **Tool Invocation**: Discrete actions with defined inputs/outputs
- **Resource Reading**: Point-in-time data retrieval

CDC requires:
- **Push-Based Streaming**: Server pushes changes as they occur
- **Persistent Connections**: Long-lived connections for event delivery
- **Ordered Event Delivery**: Maintaining transaction order and at-least-once semantics

### 2. Architectural Mismatch
CDC systems like Debezium operate at the database log level:
- PostgreSQL: WAL (Write-Ahead Log) / Logical Replication
- MySQL: Binary Log (binlog)
- SQL Server: Transaction Log / CDC tables

MCP servers operate at the query layer, which doesn't have access to these low-level change streams.

### 3. Use Case Differences
| Use Case | MCP Database Servers | CDC Systems |
|----------|---------------------|-------------|
| Ad-hoc queries | ✅ | ❌ |
| Schema exploration | ✅ | ❌ |
| Point-in-time reads | ✅ | ❌ |
| Real-time change streaming | ❌ | ✅ |
| Event-driven architectures | ❌ | ✅ |
| Data replication | ❌ | ✅ |

---

## Potential Solutions

### Option 1: Dedicated CDC MCP Server (Does Not Exist Yet)

A hypothetical CDC MCP server could expose tools like:
- `subscribe_changes(table, filters)` - Start listening for changes
- `get_change_events(subscription_id, since_lsn)` - Poll for new events
- `list_subscriptions()` - List active subscriptions

**Challenges:**
- MCP doesn't have native streaming support (SSE is one-way)
- Would require polling pattern, losing real-time benefits
- Complex state management for subscriptions

### Option 2: Kafka MCP Server + Debezium

**Architecture:**
```
Database → Debezium → Kafka → Kafka MCP Server → AI Agent
```

**Existing Components:**
- [Debezium](https://github.com/debezium/debezium) - CDC platform
- [Kafka MCP Server](https://github.com/search?q=kafka+mcp+server) - May exist, needs research

**Benefits:**
- Proven CDC architecture
- Decoupled from database
- Scalable and fault-tolerant

### Option 3: Database-Native CDC + MCP Wrapper

For PostgreSQL, a potential approach:
```sql
-- Create logical replication slot
SELECT pg_create_logical_replication_slot('mcp_slot', 'pgoutput');

-- MCP tool could poll changes
SELECT * FROM pg_logical_slot_get_changes('mcp_slot', NULL, NULL);
```

**MCP Tool Design:**
```json
{
  "name": "poll_changes",
  "parameters": {
    "slot_name": "string",
    "max_changes": "integer"
  }
}
```

### Option 4: Hybrid Architecture

Use existing MCP servers for queries + separate CDC pipeline:

```
┌─────────────────────────────────────────────────────────────┐
│                        AI Agent                             │
├────────────────────────┬────────────────────────────────────┤
│    Query Operations    │      Change Notifications          │
│         (MCP)          │         (Webhooks/Events)          │
├────────────────────────┼────────────────────────────────────┤
│    DBHub / Postgres    │    Debezium → Kafka → Webhook      │
│       MCP Pro          │                                    │
├────────────────────────┴────────────────────────────────────┤
│                      Database                               │
└─────────────────────────────────────────────────────────────┘
```

---

## Recommended Approach for TAS

Given TAS already has Kafka (`tas-kafka-shared`) in the infrastructure:

### Phase 1: Query-Based MCP (Current)
Deploy DBHub and Postgres MCP Pro for query-based database access.

### Phase 2: CDC Pipeline (Future)
1. Deploy Debezium to monitor `tas-postgres-shared`
2. Stream changes to `tas-kafka-shared`
3. Either:
   - Build custom Kafka MCP server
   - Use webhook integration for AI agents
   - Integrate with TAS Agent Builder for event-driven agents

### Phase 3: Unified MCP Interface (Future)
Consider building a TAS-specific MCP server that:
- Wraps DBHub for query operations
- Integrates Kafka consumer for CDC events
- Provides unified database interaction for AI agents

---

## Related CDC Technologies

### Debezium
- **Repository:** https://github.com/debezium/debezium
- **Supports:** PostgreSQL, MySQL, MongoDB, SQL Server, Oracle, Cassandra, Db2
- **Output:** Kafka, Redis Streams, Amazon Kinesis, Google Pub/Sub
- **License:** Apache-2.0

### PostgreSQL Native CDC
- **Logical Replication:** Built-in since PostgreSQL 10
- **pgoutput:** Native output plugin
- **wal2json:** JSON output plugin

### MySQL Native CDC
- **Binary Log:** Row-based replication events
- **Maxwell:** Open-source binlog reader
- **Canal:** Alibaba's MySQL binlog parser

---

## Conclusion

CDC support is a gap in the current MCP database server ecosystem. For TAS:

1. **Short-term:** Use MCP servers for query operations (no CDC)
2. **Medium-term:** Evaluate Kafka MCP server options or build custom
3. **Long-term:** Consider contributing CDC capabilities to the MCP ecosystem

This represents an opportunity for TAS to potentially contribute an open-source CDC MCP server to the community.

---

## Sources

- [Debezium GitHub](https://github.com/debezium/debezium)
- [MCP Specification](https://modelcontextprotocol.io/specification/)
- [PostgreSQL Logical Replication](https://www.postgresql.org/docs/current/logical-replication.html)
- [Confluent CDC Guide](https://www.confluent.io/learn/change-data-capture/)
