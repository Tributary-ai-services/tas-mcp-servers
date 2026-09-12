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
 *
 * Default connection env vars (used when no _connection is provided):
 *   (Set directly in the environment, e.g. NEO4J_URI, DATABASE_URL, etc.)
 */

import http from 'node:http';
import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';

const BRIDGE_PORT = parseInt(process.env.BRIDGE_PORT || '8000', 10);
const MCP_COMMAND = process.env.MCP_COMMAND || '';
const MCP_ARGS = JSON.parse(process.env.MCP_ARGS || '[]');
const MCP_SERVER_TYPE = process.env.MCP_SERVER_TYPE || '';
const MCP_HTTP_UPSTREAM = process.env.MCP_HTTP_UPSTREAM || '';

// ─── Connection → env var mappers ────────────────────────────────────────────

/**
 * Given a _connection object and the MCP_SERVER_TYPE, return env var overrides
 * for the child MCP server process.
 */
function connectionToEnv(conn, serverType) {
  if (!conn) return {};
  switch (serverType) {
    case 'neo4j': {
      const proto = conn.protocol || 'bolt';
      return {
        NEO4J_URI: `${proto}://${conn.host}:${conn.port}`,
        NEO4J_USERNAME: conn.username,
        NEO4J_PASSWORD: conn.password,
        NEO4J_DATABASE: conn.database || 'neo4j',
      };
    }
    case 'postgres': {
      const ssl = conn.ssl_mode ? `?sslmode=${conn.ssl_mode}` : '';
      return {
        DATABASE_URL: `postgresql://${conn.username}:${encodeURIComponent(conn.password)}@${conn.host}:${conn.port}/${conn.database}${ssl}`,
      };
    }
    case 'minio': {
      return {
        MINIO_ENDPOINT: `http://${conn.host}:${conn.port}`,
        MINIO_ACCESS_KEY: conn.username,
        MINIO_SECRET_KEY: conn.password,
      };
    }
    case 'kafka': {
      return {
        KAFKA_BROKERS: `${conn.host}:${conn.port}`,
        KAFKA_USERNAME: conn.username || '',
        KAFKA_PASSWORD: conn.password || '',
      };
    }
    case 'grafana': {
      const proto = conn.protocol || 'https';
      return {
        GRAFANA_URL: `${proto}://${conn.host}:${conn.port}`,
        GRAFANA_API_KEY: conn.password || '',
      };
    }
    default:
      return {};
  }
}

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

// ─── MCP stdio process manager ───────────────────────────────────────────────

/**
 * Manages a pool of MCP server child processes.
 * - Default process uses env vars from the container
 * - Connection-specific processes get env var overrides
 */
class MCPProcessManager {
  constructor() {
    // Cache: connectionKey -> { process, pending, lastUsed }
    this.processes = new Map();
  }

  _connectionKey(conn) {
    if (!conn) return '__default__';
    return `${conn.host}:${conn.port}:${conn.database || ''}:${conn.username || ''}`;
  }

  _buildChildEnv(conn) {
    // Start with parent env, strip MCP_ENV_ prefix for forwarded vars
    const env = { ...process.env };
    for (const [key, val] of Object.entries(process.env)) {
      if (key.startsWith('MCP_ENV_')) {
        env[key.slice(8)] = val;
      }
    }
    // Apply connection overrides
    const overrides = connectionToEnv(conn, MCP_SERVER_TYPE);
    Object.assign(env, overrides);
    return env;
  }

  _spawn(conn) {
    const env = this._buildChildEnv(conn);
    const [cmd, ...defaultArgs] = MCP_COMMAND.split(/\s+/);
    const args = [...defaultArgs, ...MCP_ARGS];

    console.error(`[bridge] Spawning: ${cmd} ${args.join(' ')} (type=${MCP_SERVER_TYPE})`);

    const child = spawn(cmd, args, {
      env,
      stdio: ['pipe', 'pipe', 'inherit'],
    });

    child.on('error', (err) => {
      console.error(`[bridge] Child process error: ${err.message}`);
    });
    child.on('exit', (code) => {
      console.error(`[bridge] Child process exited with code ${code}`);
      const key = this._connectionKey(conn);
      this.processes.delete(key);
    });

    return {
      process: child,
      pending: new Map(), // id -> { resolve, reject }
      buffer: '',
      lastUsed: Date.now(),
      initPromise: null, // memoized MCP initialize handshake
    };
  }

  getOrSpawn(conn) {
    const key = this._connectionKey(conn);
    let entry = this.processes.get(key);
    if (!entry || entry.process.killed) {
      entry = this._spawn(conn);
      this.processes.set(key, entry);
      this._setupLineReader(entry);
    }
    entry.lastUsed = Date.now();
    return entry;
  }

  _setupLineReader(entry) {
    entry.process.stdout.on('data', (chunk) => {
      entry.buffer += chunk.toString();
      let newlineIdx;
      while ((newlineIdx = entry.buffer.indexOf('\n')) !== -1) {
        const line = entry.buffer.slice(0, newlineIdx).trim();
        entry.buffer = entry.buffer.slice(newlineIdx + 1);
        if (!line) continue;
        try {
          const msg = JSON.parse(line);
          if (msg.id != null && entry.pending.has(msg.id)) {
            const { resolve } = entry.pending.get(msg.id);
            entry.pending.delete(msg.id);
            resolve(msg);
          }
        } catch {
          // Not JSON, ignore
        }
      }
    });
  }

  async sendRequest(conn, method, params) {
    const entry = this.getOrSpawn(conn);
    // The MCP spec requires an initialize handshake before any other request.
    // Strict servers (e.g. the Python mcp-neo4j-cypher) reject tools/list with
    // "Received request before initialization was complete" otherwise.
    await this._ensureInitialized(entry);
    return this._rawRequest(entry, method, params);
  }

  // _ensureInitialized runs (once per child) the initialize request + the
  // initialized notification, memoizing the in-flight handshake so concurrent
  // callers await the same one.
  _ensureInitialized(entry) {
    if (!entry.initPromise) {
      entry.initPromise = (async () => {
        await this._rawRequest(entry, 'initialize', {
          protocolVersion: '2024-11-05',
          capabilities: {},
          clientInfo: {
            name: process.env.BRIDGE_NAME || 'tas-mcp-bridge',
            version: process.env.BRIDGE_VERSION || '1.0.0',
          },
        });
        this._notify(entry, 'notifications/initialized', {});
      })().catch((err) => {
        // Reset so the next request retries the handshake rather than wedging.
        entry.initPromise = null;
        throw err;
      });
    }
    return entry.initPromise;
  }

  // _notify writes a JSON-RPC notification (no id, no response expected).
  _notify(entry, method, params) {
    entry.process.stdin.write(JSON.stringify({ jsonrpc: '2.0', method, params }) + '\n');
  }

  _rawRequest(entry, method, params) {
    const id = randomUUID();
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        entry.pending.delete(id);
        reject(new Error('MCP request timeout (60s)'));
      }, 60000);

      entry.pending.set(id, {
        resolve: (msg) => {
          clearTimeout(timeout);
          resolve(msg);
        },
        reject: (err) => {
          clearTimeout(timeout);
          reject(err);
        },
      });

      entry.process.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
    });
  }
}

// ─── Select backend mode ────────────────────────────────────────────────────

const useHttpUpstream = !!MCP_HTTP_UPSTREAM;
const httpProxy = useHttpUpstream ? new MCPHttpUpstreamProxy(MCP_HTTP_UPSTREAM) : null;
const processManager = useHttpUpstream ? null : new MCPProcessManager();

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

async function handleCallTool(body) {
  const { name, arguments: args } = JSON.parse(body);
  const cleanArgs = { ...args };

  // Extract _connection if present
  const conn = cleanArgs._connection || null;
  delete cleanArgs._connection;

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

  const resp = await processManager.sendRequest(conn, 'tools/call', {
    name,
    arguments: cleanArgs,
  });

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
      const result = await handleCallTool(body);
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

if (!MCP_COMMAND && !MCP_HTTP_UPSTREAM) {
  console.error('[bridge] ERROR: Either MCP_COMMAND or MCP_HTTP_UPSTREAM env var is required');
  process.exit(1);
}

server.listen(BRIDGE_PORT, () => {
  console.error(`[bridge] TAS MCP Bridge v2.1 listening on port ${BRIDGE_PORT}`);
  console.error(`[bridge] Server type: ${MCP_SERVER_TYPE || '(none)'}`);
  if (useHttpUpstream) {
    console.error(`[bridge] Mode: HTTP upstream proxy → ${MCP_HTTP_UPSTREAM}`);
  } else {
    console.error(`[bridge] Mode: stdio process`);
    console.error(`[bridge] Command: ${MCP_COMMAND} ${MCP_ARGS.join(' ')}`);
  }
});
