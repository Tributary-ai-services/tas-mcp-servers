/**
 * TAS MCP Bridge v2.1 — Connection-Aware HTTP wrapper for MCP servers.
 *
 * This bridge:
 *  1. Wraps an underlying MCP server (stdio OR HTTP upstream)
 *  2. Exposes HTTP endpoints: GET /health, GET /mcp/tools/list, POST /mcp/tools/call
 *  3. Intercepts `_connection` in tool arguments and rebuilds the connection env vars
 *     for the underlying server based on server type
 *
 * Environment variables:
 *   MCP_COMMAND        - Command to run the underlying MCP server via stdio (e.g. "npx @modelcontextprotocol/server-postgres")
 *   MCP_ARGS           - JSON array of additional arguments (optional)
 *   MCP_HTTP_UPSTREAM  - URL of an upstream HTTP MCP server (e.g. "http://localhost:8090/mcp")
 *                        When set, MCP_COMMAND is ignored and the bridge proxies via Streamable HTTP transport
 *   MCP_SERVER_TYPE    - Server type for connection mapping: neo4j|postgres|minio|kafka|grafana
 *   BRIDGE_PORT        - HTTP port (default: 8000)
 *   MCP_ENV_*          - Extra env vars forwarded to child process (strip MCP_ENV_ prefix)
 *   BRIDGE_MAX_CHILDREN - Most connection-specific child processes kept (default 32, LRU-evicted)
 *   BRIDGE_DSN_EGRESS  - "enforce" checks every _connection's host:port before dialing
 *                        (MCP-19, src/egress.js); unset = off. With it:
 *   MCP_DSN_EGRESS_ALLOW, MCP_TENANT_EGRESS_ALLOWLIST, MCP_TENANT_CLUSTER_NAMESPACES
 *                      - the same values the tas-mcp proxy and aiqg-dashboard-be read
 *
 * Child processes are keyed by a hash of the whole _connection (password
 * included) plus the TAS-Tenant-Id the tas-mcp proxy sends; see src/pool.js.
 *
 * Default connection env vars (used when no _connection is provided):
 *   (Set directly in the environment, e.g. NEO4J_URI, DATABASE_URL, etc.)
 */

import http from 'node:http';
import { randomUUID } from 'node:crypto';
import { pathToFileURL } from 'node:url';

import { MCPProcessManager, tenantOf } from './pool.js';
import { EgressError, guardConnection, loadEgressRule } from './egress.js';

const BRIDGE_PORT = parseInt(process.env.BRIDGE_PORT || '8000', 10);
const MCP_COMMAND = process.env.MCP_COMMAND || '';
const MCP_ARGS = JSON.parse(process.env.MCP_ARGS || '[]');
const MCP_SERVER_TYPE = process.env.MCP_SERVER_TYPE || '';
const MCP_HTTP_UPSTREAM = process.env.MCP_HTTP_UPSTREAM || '';
// MCP-19: null unless BRIDGE_DSN_EGRESS=enforce. Config errors fail closed.
const EGRESS_RULE = loadEgressRule(process.env);

// ─── HTTP upstream proxy (Streamable HTTP MCP transport) ─────────────────────

/**
 * Manages sessions with an upstream Streamable HTTP MCP server.
 * Handles initialize handshake and session ID tracking.
 */
class MCPHttpUpstreamProxy {
  constructor(upstreamUrl) {
    this.upstreamUrl = upstreamUrl;
    this.sessionId = null;
    this.initializing = null; // promise for in-flight init
  }

  async _httpPost(body, headers = {}) {
    const url = new URL(this.upstreamUrl);
    const reqBody = JSON.stringify(body);

    const reqHeaders = {
      'Content-Type': 'application/json',
      'Accept': 'application/json, text/event-stream',
      ...headers,
    };

    return new Promise((resolve, reject) => {
      const options = {
        hostname: url.hostname,
        port: url.port || (url.protocol === 'https:' ? 443 : 80),
        path: url.pathname,
        method: 'POST',
        headers: {
          ...reqHeaders,
          'Content-Length': Buffer.byteLength(reqBody),
        },
      };

      const req = http.request(options, (res) => {
        let data = '';
        res.on('data', (chunk) => (data += chunk));
        res.on('end', () => {
          resolve({
            status: res.statusCode,
            headers: res.headers,
            body: data,
          });
        });
      });

      req.on('error', reject);
      req.setTimeout(30000, () => {
        req.destroy(new Error('HTTP upstream timeout (30s)'));
      });
      req.write(reqBody);
      req.end();
    });
  }

  async _ensureSession() {
    if (this.sessionId) return;

    // Prevent concurrent initializations
    if (this.initializing) {
      await this.initializing;
      return;
    }

    this.initializing = this._doInitialize();
    try {
      await this.initializing;
    } finally {
      this.initializing = null;
    }
  }

  async _doInitialize() {
    console.error(`[bridge] Initializing session with upstream: ${this.upstreamUrl}`);

    const initBody = {
      jsonrpc: '2.0',
      id: randomUUID(),
      method: 'initialize',
      params: {
        protocolVersion: '2025-03-26',
        capabilities: {},
        clientInfo: { name: 'tas-mcp-bridge', version: '2.1.0' },
      },
    };

    const resp = await this._httpPost(initBody);

    if (resp.status !== 200) {
      throw new Error(`Upstream initialize failed (${resp.status}): ${resp.body}`);
    }

    // Extract session ID from response header
    const sid = resp.headers['mcp-session-id'];
    if (sid) {
      this.sessionId = sid;
      console.error(`[bridge] Session established: ${sid}`);
    } else {
      console.error('[bridge] Warning: upstream did not return Mcp-Session-Id, proceeding without session');
    }

    // Parse initialize result
    try {
      const result = JSON.parse(resp.body);
      if (result.error) {
        throw new Error(`Initialize error: ${result.error.message || JSON.stringify(result.error)}`);
      }
      console.error(`[bridge] Upstream server: ${result.result?.serverInfo?.name} v${result.result?.serverInfo?.version}`);
    } catch (e) {
      if (e.message.startsWith('Initialize error')) throw e;
      // JSON parse error — just log
      console.error(`[bridge] Could not parse initialize response: ${resp.body.slice(0, 200)}`);
    }

    // Send initialized notification
    const notif = { jsonrpc: '2.0', method: 'notifications/initialized', params: {} };
    const headers = this.sessionId ? { 'Mcp-Session-Id': this.sessionId } : {};
    await this._httpPost(notif, headers).catch((err) => {
      console.error(`[bridge] Warning: initialized notification failed: ${err.message}`);
    });
  }

  async sendRequest(method, params) {
    await this._ensureSession();

    const body = {
      jsonrpc: '2.0',
      id: randomUUID(),
      method,
      params,
    };

    const headers = this.sessionId ? { 'Mcp-Session-Id': this.sessionId } : {};
    const resp = await this._httpPost(body, headers);

    // If we get 400 "Invalid session ID", re-initialize and retry once
    if (resp.status === 400 && resp.body.includes('session')) {
      console.error('[bridge] Session expired, re-initializing...');
      this.sessionId = null;
      await this._ensureSession();
      const retryHeaders = this.sessionId ? { 'Mcp-Session-Id': this.sessionId } : {};
      const retryResp = await this._httpPost(body, retryHeaders);
      if (retryResp.status !== 200) {
        throw new Error(`Upstream error (${retryResp.status}): ${retryResp.body}`);
      }
      return JSON.parse(retryResp.body);
    }

    if (resp.status !== 200) {
      throw new Error(`Upstream error (${resp.status}): ${resp.body}`);
    }

    return JSON.parse(resp.body);
  }
}

// ─── Select backend mode ────────────────────────────────────────────────────

const useHttpUpstream = !!MCP_HTTP_UPSTREAM;
const httpProxy = useHttpUpstream ? new MCPHttpUpstreamProxy(MCP_HTTP_UPSTREAM) : null;
const processManager = useHttpUpstream ? null : new MCPProcessManager({
  command: MCP_COMMAND, args: MCP_ARGS, serverType: MCP_SERVER_TYPE,
  maxChildren: parseInt(process.env.BRIDGE_MAX_CHILDREN || '32', 10),
});

// ─── HTTP server ─────────────────────────────────────────────────────────────

function readBody(req) {
  return new Promise((resolve, reject) => {
    let data = '';
    req.on('data', (chunk) => (data += chunk));
    req.on('end', () => resolve(data));
    req.on('error', reject);
  });
}

async function handleListTools() {
  if (useHttpUpstream) {
    const resp = await httpProxy.sendRequest('tools/list', {});
    if (resp.error) throw new Error(resp.error.message || JSON.stringify(resp.error));
    return { tools: resp.result?.tools || [] };
  }

  const resp = await processManager.sendRequest(null, 'tools/list', {});
  if (resp.error) throw new Error(resp.error.message || JSON.stringify(resp.error));
  return { tools: resp.result?.tools || [] };
}

export async function handleCallTool(body, tenant, { rule = EGRESS_RULE, lookup } = {}) {
  const { name, arguments: args } = JSON.parse(body);
  const cleanArgs = { ...args };

  // Extract _connection if present
  let conn = cleanArgs._connection || null;
  delete cleanArgs._connection;

  // MCP-19: check the host:port (and the addresses it resolves to) before any
  // child dials it, and dial the checked address. A refusal never starts a
  // child and never echoes the connection.
  if (rule && conn) {
    try {
      conn = await guardConnection(rule, conn, tenant, lookup ? { lookup } : {});
    } catch (err) {
      if (!(err instanceof EgressError)) throw err;
      console.error(`[bridge] ${err.message}`);
      return { content: [{ type: 'text', text: `Error: ${err.message}` }], isError: true };
    }
  }

  if (useHttpUpstream) {
    const resp = await httpProxy.sendRequest('tools/call', {
      name,
      arguments: cleanArgs,
    });

    if (resp.error) {
      return { content: [{ type: 'text', text: `Error: ${resp.error.message || JSON.stringify(resp.error)}` }], isError: true };
    }
    return resp.result || { content: [{ type: 'text', text: 'No result' }] };
  }

  // The child is chosen by the whole connection (password included) and the
  // tenant the tas-mcp proxy names, never by host/port/database/user alone.
  const resp = await processManager.sendRequest(conn, 'tools/call', {
    name,
    arguments: cleanArgs,
  }, tenant);

  if (resp.error) {
    return { content: [{ type: 'text', text: `Error: ${resp.error.message || JSON.stringify(resp.error)}` }], isError: true };
  }

  return resp.result || { content: [{ type: 'text', text: 'No result' }] };
}

const server = http.createServer(async (req, res) => {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, X-Space-ID, X-Space-Type, X-Tenant-ID');

  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }

  try {
    if (req.url === '/health' && req.method === 'GET') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        status: 'healthy',
        service: 'tas-mcp-bridge',
        version: '2.1.0',
        serverType: MCP_SERVER_TYPE,
        mode: useHttpUpstream ? 'http-upstream' : 'stdio',
        timestamp: new Date().toISOString(),
      }));
    } else if (req.url === '/mcp/tools/list' && req.method === 'GET') {
      const result = await handleListTools();
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(result));
    } else if (req.url === '/mcp/tools/call' && req.method === 'POST') {
      const body = await readBody(req);
      const result = await handleCallTool(body, tenantOf(req.headers));
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(result));
    } else {
      res.writeHead(404, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Not found' }));
    }
  } catch (err) {
    console.error(`[bridge] Error: ${err.message}`);
    res.writeHead(500, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: err.message }));
  }
});

const isMain = process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href;

if (isMain && !MCP_COMMAND && !MCP_HTTP_UPSTREAM) {
  console.error('[bridge] ERROR: Either MCP_COMMAND or MCP_HTTP_UPSTREAM env var is required');
  process.exit(1);
}

if (isMain) server.listen(BRIDGE_PORT, () => {
  console.error(`[bridge] TAS MCP Bridge v2.1 listening on port ${BRIDGE_PORT}`);
  console.error(`[bridge] Server type: ${MCP_SERVER_TYPE || '(none)'}`);
  if (EGRESS_RULE) {
    for (const e of EGRESS_RULE.errors) console.error(`[bridge] DSN egress config ignored (fail closed): ${e}`);
    console.error(`[bridge] DSN egress: enforce (${EGRESS_RULE.platform.length} platform pair(s), ` +
      `namespaces ${EGRESS_RULE.namespaces.join(',')}, ${Object.keys(EGRESS_RULE.tenants).length} tenant list(s))`);
  } else {
    console.error('[bridge] DSN egress: off (BRIDGE_DSN_EGRESS unset)');
  }
  if (useHttpUpstream) {
    console.error(`[bridge] Mode: HTTP upstream proxy → ${MCP_HTTP_UPSTREAM}`);
  } else {
    console.error(`[bridge] Mode: stdio process`);
    console.error(`[bridge] Command: ${MCP_COMMAND} ${MCP_ARGS.join(' ')}`);
  }
});
