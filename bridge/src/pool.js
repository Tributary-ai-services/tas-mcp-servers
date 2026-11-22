/**
 * MCP stdio process pool for the TAS MCP bridge.
 *
 * Each distinct connection gets its own child MCP server process, started with
 * that connection's credentials in its environment. A child is reused only for
 * a caller presenting the *same* connection.
 *
 * Isolation (security review, 2026-11-21): children used to be keyed by
 * `host:port:database:username` alone, with no password and no tenant, so any
 * caller who sent `_connection` with another tenant's host, port, database and
 * user, and any password at all, was served by that tenant's already-running
 * child, authenticated as that tenant. Children are now keyed by a SHA-256 of
 * the whole connection, password included, plus the tenant the tas-mcp proxy
 * names in `TAS-Tenant-Id` when it injected the connection. A child whose
 * credentials changed (a rotation: same tenant and database identity, new
 * secret) is killed and replaced, so the old secret stops working at once.
 */

import { spawn as nodeSpawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';

/**
 * Given a _connection object and the MCP_SERVER_TYPE, return env var overrides
 * for the child MCP server process.
 */
export function connectionToEnv(conn, serverType) {
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

/** The default (no _connection) child's key. */
export const DEFAULT_KEY = '__default__';

/** Canonical JSON: object keys sorted at every level. */
function canonical(v) {
  if (Array.isArray(v)) return `[${v.map(canonical).join(',')}]`;
  if (v && typeof v === 'object') {
    return `{${Object.keys(v).sort().map((k) => `${JSON.stringify(k)}:${canonical(v[k])}`).join(',')}}`;
  }
  return JSON.stringify(v === undefined ? null : v);
}

/**
 * connectionKey is the pool key of a connection: a SHA-256 over the tenant and
 * the whole connection, password included. Two callers share a child only when
 * they present the same tenant and exactly the same credentials. The key holds
 * no plaintext.
 */
export function connectionKey(conn, tenant = '') {
  if (!conn) return DEFAULT_KEY;
  return createHash('sha256').update(`${tenant || ''}\u0000${canonical(conn)}`).digest('hex');
}

/**
 * slotKey names "the same database login for the same tenant", without the
 * secret: when a new key arrives for an existing slot the credentials changed
 * (a rotation), and the old child is retired. Hashed, so it holds no plaintext.
 */
export function slotKey(conn, tenant = '') {
  if (!conn) return DEFAULT_KEY;
  const id = `${tenant || ''}\u0000${conn.host}:${conn.port}:${conn.database || ''}:${conn.username || ''}`;
  return createHash('sha256').update(id).digest('hex');
}

/** DEFAULT_MAX_CHILDREN bounds the pool; the least recently used is evicted. */
export const DEFAULT_MAX_CHILDREN = 32;

/**
 * Manages a pool of MCP server child processes.
 * - The default process uses env vars from the container.
 * - Connection-specific processes get env var overrides.
 */
export class MCPProcessManager {
  /**
   * @param {object} opts
   * @param {string} opts.command      MCP_COMMAND
   * @param {string[]} opts.args       MCP_ARGS
   * @param {string} opts.serverType   MCP_SERVER_TYPE
   * @param {Function} [opts.spawn]    child_process.spawn (injectable for tests)
   * @param {number} [opts.maxChildren]
   * @param {object} [opts.env]        base environment (default process.env)
   */
  constructor({ command = '', args = [], serverType = '', spawn = nodeSpawn, maxChildren = DEFAULT_MAX_CHILDREN, env = process.env } = {}) {
    this.command = command;
    this.args = args;
    this.serverType = serverType;
    this.spawnFn = spawn;
    this.maxChildren = maxChildren;
    this.baseEnv = env;
    // key -> { process, pending, buffer, lastUsed, initPromise, slot }
    this.processes = new Map();
    // slot -> key of the child currently serving it
    this.slots = new Map();
  }

  _buildChildEnv(conn) {
    // Start with parent env, strip MCP_ENV_ prefix for forwarded vars
    const env = { ...this.baseEnv };
    for (const [key, val] of Object.entries(this.baseEnv)) {
      if (key.startsWith('MCP_ENV_')) {
        env[key.slice(8)] = val;
      }
    }
    Object.assign(env, connectionToEnv(conn, this.serverType));
    return env;
  }

  _spawn(conn, key) {
    const env = this._buildChildEnv(conn);
    const [cmd, ...defaultArgs] = this.command.split(/\s+/);
    const args = [...defaultArgs, ...this.args];

    // Never log the connection: it holds the password.
    console.error(`[bridge] Spawning: ${cmd} ${args.join(' ')} (type=${this.serverType}, pool=${this.processes.size + 1})`);

    const child = this.spawnFn(cmd, args, { env, stdio: ['pipe', 'pipe', 'inherit'] });
    const entry = {
      process: child,
      pending: new Map(), // id -> { resolve, reject }
      buffer: '',
      lastUsed: Date.now(),
      initPromise: null, // memoized MCP initialize handshake
      slot: null,
    };
    child.on('error', (err) => {
      console.error(`[bridge] Child process error: ${err.message}`);
    });
    child.on('exit', (code) => {
      console.error(`[bridge] Child process exited with code ${code}`);
      // Only forget this child: a replacement may already hold the key.
      if (this.processes.get(key) === entry) this.processes.delete(key);
      if (entry.slot && this.slots.get(entry.slot) === key) this.slots.delete(entry.slot);
      for (const { reject } of entry.pending.values()) reject(new Error('MCP child exited'));
      entry.pending.clear();
    });
    return entry;
  }

  _retire(key) {
    const entry = this.processes.get(key);
    if (!entry) return;
    this.processes.delete(key);
    if (entry.slot && this.slots.get(entry.slot) === key) this.slots.delete(entry.slot);
    try {
      entry.process.kill();
    } catch {
      // already gone
    }
  }

  /**
   * getOrSpawn returns the child for exactly this connection and tenant,
   * starting one if needed. A different secret for the same slot retires the
   * old child (rotation).
   */
  getOrSpawn(conn, tenant = '') {
    const key = connectionKey(conn, tenant);
    let entry = this.processes.get(key);
    if (!entry || entry.process.killed) {
      const slot = conn ? slotKey(conn, tenant) : null;
      if (slot) {
        const prev = this.slots.get(slot);
        if (prev && prev !== key) this._retire(prev); // credentials changed
      }
      while (this.processes.size >= this.maxChildren) this._evictLRU();
      entry = this._spawn(conn, key);
      entry.slot = slot;
      this.processes.set(key, entry);
      if (slot) this.slots.set(slot, key);
      this._setupLineReader(entry);
    }
    entry.lastUsed = Date.now();
    return entry;
  }

  _evictLRU() {
    let oldest = null;
    for (const [k, e] of this.processes) {
      if (k === DEFAULT_KEY) continue;
      if (!oldest || e.lastUsed < oldest[1].lastUsed) oldest = [k, e];
    }
    if (!oldest) {
      // Only the default is left; let it be.
      this.maxChildren = this.processes.size + 1;
      return;
    }
    this._retire(oldest[0]);
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

  async sendRequest(conn, method, params, tenant = '') {
    const entry = this.getOrSpawn(conn, tenant);
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

/** The tenant header the tas-mcp proxy sends with an injected DSN. */
export const TENANT_HEADER = 'tas-tenant-id';

/** tenantOf reads the proxy-supplied tenant from request headers (or ''). */
export function tenantOf(headers) {
  const v = headers?.[TENANT_HEADER];
  const t = (Array.isArray(v) ? v[0] : v || '').trim();
  return /^[0-9a-fA-F-]{1,64}$/.test(t) ? t.toLowerCase() : '';
}
