/**
 * DSN egress for the TAS MCP bridge (MCP-19), checked when the bridge is
 * about to dial a `_connection`.
 *
 * The tas-mcp proxy checks a stored DSN's host and port before it injects it
 * (tas-mcp internal/federation/dsnegress.go, DSNRule), and aiqg-dashboard-be
 * checks it before it stores it (internal/mcpconfig/egress.go). The bridge is
 * where the TCP connection is made, so it applies the same rule again (any pod
 * can call the bridge's Service directly, not only the proxy) and then does
 * the one thing the others cannot: it resolves the name, checks every
 * address, and hands the database the address it checked, so a DNS answer
 * that changes between the check and the dial (rebinding) cannot reach the
 * cluster.
 *
 * Every field of the _connection is validated first (`checkFields`): only
 * host, port, username, password, database, protocol and ssl_mode; username
 * and database of [A-Za-z0-9._-]; port an integer; ssl_mode from a fixed set.
 * A user name or database could otherwise rewrite the URL the child dials.
 *
 * The host:port rule (`checkHost`), in order:
 *   0. The host must be plain (`hostSyntaxOK`).
 *   1. Always refused, even when listed: localhost names, loopback, link-local
 *      (169.254.169.254 included), fd00:ec2::254, unspecified and multicast
 *      addresses, and non-standard numeric hosts ("2130706433", "127.1").
 *   2. Allowed: a pair on MCP_DSN_EGRESS_ALLOW (kind "platform").
 *   3. Allowed: a service in MCP_TENANT_CLUSTER_NAMESPACES (default
 *      tas-mcp-servers; kind "cluster"). Other cluster names, bare names: no.
 *   4. An IP literal passes only when the tenant's MCP_TENANT_EGRESS_ALLOWLIST
 *      lists it and it is public (kind "tenant"); a private one only through
 *      MCP_DSN_EGRESS_ALLOW.
 *   5. Allowed: a name on the tenant's allowlist; "host:port" matches that
 *      port only (kind "tenant").
 *
 * The dial rule (`checkAddresses`) over the resolved addresses:
 *   - never loopback, link-local, metadata, unspecified, 0.0.0.0/8 or
 *     multicast, for any kind;
 *   - kind "tenant": public addresses only (no RFC 1918, 100.64/10, fc00::/7),
 *     because the tenant list names hosts outside the cluster;
 *   - kinds "platform" and "cluster" may resolve to a ClusterIP.
 *
 * test/dsn_egress_vectors.json is the same file as in tas-mcp and
 * aiqg-dashboard-be, pinned by sha256 in all three.
 *
 * Off unless BRIDGE_DSN_EGRESS=enforce: the bridge also fronts neo4j, minio,
 * kafka and grafana servers whose callers pass other hosts today.
 */

import { promises as dns } from 'node:dns';
import net from 'node:net';

// ─── addresses ──────────────────────────────────────────────────────────────

/** parseIPv6 returns the 16 bytes of an IPv6 literal (no zone), or null. */
function parseIPv6(s) {
  if (net.isIP(s) !== 6) return null;
  let str = s;
  const lastColon = str.lastIndexOf(':');
  const v4 = str.slice(lastColon + 1);
  if (v4.includes('.')) {
    // Write an embedded dotted quad (::ffff:1.2.3.4) as two hex groups.
    const b = parseIPv4(v4);
    if (!b) return null;
    str = `${str.slice(0, lastColon + 1)}${((b[0] << 8) | b[1]).toString(16)}:${((b[2] << 8) | b[3]).toString(16)}`;
  }
  let groups;
  if (str.includes('::')) {
    const [l, r] = str.split('::');
    const left = l ? l.split(':') : [];
    const right = r ? r.split(':') : [];
    groups = [...left, ...Array(8 - left.length - right.length).fill('0'), ...right];
  } else {
    groups = str.split(':');
  }
  const words = groups.map((g) => parseInt(g, 16));
  if (words.length !== 8 || words.some((w) => Number.isNaN(w) || w < 0 || w > 0xffff)) return null;
  const out = [];
  for (const w of words) out.push(w >> 8, w & 0xff);
  return out;
}

/** parseIPv4 returns the 4 bytes of a dotted-quad literal, or null. */
function parseIPv4(s) {
  if (net.isIP(s) !== 4) return null;
  return s.split('.').map((x) => parseInt(x, 10));
}

/**
 * parseAddr reads an IP literal (brackets and a zone allowed) into
 * { v: 4|6, b: bytes }, unmapping an IPv4-mapped IPv6 address to IPv4.
 * Returns null for anything that is not a standard IP literal.
 */
export function parseAddr(raw) {
  let s = String(raw).trim();
  if (s.startsWith('[') && s.endsWith(']')) s = s.slice(1, -1);
  const pct = s.indexOf('%');
  if (pct >= 0) s = s.slice(0, pct);
  const v4 = parseIPv4(s);
  if (v4) return { v: 4, b: v4 };
  const v6 = parseIPv6(s);
  if (!v6) return null;
  const mapped = v6.slice(0, 10).every((x) => x === 0) && v6[10] === 0xff && v6[11] === 0xff;
  if (mapped) return { v: 4, b: v6.slice(12) };
  return { v: 6, b: v6 };
}

/** formatAddr writes an address canonically (RFC 5952 for IPv6, as Go does). */
export function formatAddr(a) {
  if (a.v === 4) return a.b.join('.');
  const w = [];
  for (let i = 0; i < 16; i += 2) w.push((a.b[i] << 8) | a.b[i + 1]);
  let best = -1;
  let bestLen = 0;
  for (let i = 0; i < 8;) {
    if (w[i] !== 0) { i++; continue; }
    let j = i;
    while (j < 8 && w[j] === 0) j++;
    if (j - i > bestLen && j - i >= 2) { best = i; bestLen = j - i; }
    i = j;
  }
  const hex = w.map((x) => x.toString(16));
  if (best < 0) return hex.join(':');
  return `${hex.slice(0, best).join(':')}::${hex.slice(best + bestLen).join(':')}`;
}

function inPrefix(a, v, bytes, bits) {
  if (a.v !== v) return false;
  for (let i = 0; i < bits; i++) {
    const byte = i >> 3;
    const mask = 0x80 >> (i & 7);
    if ((a.b[byte] & mask) !== (bytes[byte] & mask)) return false;
  }
  return true;
}

const pfx = (s, bits) => { const a = parseAddr(s); return { v: a.v, b: a.b, bits }; };
const ALWAYS_DENIED = [
  pfx('127.0.0.0', 8), pfx('::1', 128), // loopback
  pfx('169.254.0.0', 16), pfx('fe80::', 10), // link-local (metadata 169.254.169.254)
  pfx('224.0.0.0', 4), pfx('ff00::', 8), // multicast (incl. link-local multicast)
  pfx('fd00:ec2::254', 128), // AWS IPv6 metadata
  pfx('0.0.0.0', 32), pfx('::', 128), // unspecified
];
const PRIVATE = [
  pfx('10.0.0.0', 8), pfx('172.16.0.0', 12), pfx('192.168.0.0', 16),
  pfx('100.64.0.0', 10), pfx('fc00::', 7),
  // NAT64: a NAT64 gateway turns 64:ff9b::a9fe:a9fe into 169.254.169.254.
  pfx('64:ff9b::', 96), pfx('64:ff9b:1::', 48),
];
// Dial time only: 0.0.0.0/8 is "this network" and dials local addresses.
const DIAL_DENIED = [...ALWAYS_DENIED, pfx('0.0.0.0', 8)];

const matchesAny = (a, list) => list.some((p) => inPrefix(a, p.v, p.b, p.bits));

/** deniedAddr is true for an address no list may open (rule 1). */
export function deniedAddr(a) { return matchesAny(a, ALWAYS_DENIED); }

/** isPrivateAddr is true for RFC 1918, RFC 6598 and IPv6 unique-local. */
export function isPrivateAddr(a) { return matchesAny(a, PRIVATE); }

// ─── names ──────────────────────────────────────────────────────────────────

/**
 * canonicalHost lowercases, drops brackets and a trailing dot, writes an IP
 * canonically, and writes <svc>.<ns>.svc as <svc>.<ns>.svc.cluster.local.
 */
export function canonicalHost(raw) {
  let h = String(raw ?? '').trim().replace(/^\[/, '').replace(/\]$/, '').toLowerCase();
  if (h.endsWith('.')) h = h.slice(0, -1);
  const a = parseAddr(h);
  if (a) return formatAddr(a);
  if (h.endsWith('.svc')) h += '.cluster.local';
  return h;
}

/** ambiguousNumeric: resolvers read "127.1", "0177.0.0.1", "2130706433" as IPs. */
function ambiguousNumeric(host) {
  const labels = host.split('.');
  const last = labels[labels.length - 1];
  if (last === '') return false;
  if (last.startsWith('0x')) return true;
  if (/^[0-9]+$/.test(last)) return true;
  return labels[0].startsWith('0x');
}

function alwaysDenied(host) {
  if (host === 'localhost' || host.endsWith('.localhost')) return `${host} is localhost`;
  const a = parseAddr(host);
  if (a) return deniedAddr(a) ? `${host} is loopback, link-local, metadata, unspecified or multicast` : '';
  if (ambiguousNumeric(host)) return `${host} looks like an IP address in a non-standard form`;
  return '';
}

function clusterNamespace(host) {
  let h = host;
  if (h.endsWith('.cluster.local')) h = h.slice(0, -'.cluster.local'.length);
  if (!h.endsWith('.svc')) return null;
  const labels = h.slice(0, -'.svc'.length).split('.');
  if (labels.length !== 2 || !labels[0] || !labels[1]) return null;
  return labels[1];
}

function isClusterInternal(host) {
  return host === 'localhost' || host.endsWith('.localhost') || host.endsWith('.svc') ||
    host.endsWith('.cluster.local') || host.endsWith('.internal');
}

const validPort = (p) => Number.isInteger(p) && p > 0 && p <= 65535;

// An LDH host name (the proxy's dnsNameRe).
const DNS_NAME_RE = /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$/;

/**
 * hostSyntaxOK is rule 0: only an LDH name, an IP literal with no zone, or one
 * pair of brackets around an IPv6 literal. The same as the proxy and AIQG.
 */
export function hostSyntaxOK(raw) {
  if (typeof raw !== 'string' || raw === '' || raw.length > 255 || raw.includes('%')) return false;
  if (raw.startsWith('[')) {
    if (!raw.endsWith(']')) return false;
    return net.isIP(raw.slice(1, -1)) === 6;
  }
  if (net.isIP(raw) !== 0) return true;
  return DNS_NAME_RE.test(raw);
}

/** splitHostPort reads "host:port" / "[v6]:port"; null when there is no port. */
function splitHostPort(s) {
  const m = /^\[([^\]]+)\]:([^:]+)$/.exec(s) || /^([^:[\]]+):([^:]+)$/.exec(s);
  if (!m) return null;
  return { host: m[1], port: /^[0-9]+$/.test(m[2]) ? parseInt(m[2], 10) : NaN };
}

function splitAllowEntry(raw) {
  const e = String(raw ?? '').trim();
  if (!e) return null;
  const hp = splitHostPort(e);
  if (hp) return validPort(hp.port) ? { host: canonicalHost(hp.host), port: hp.port } : null;
  return { host: canonicalHost(e), port: 0 };
}

// ─── config ─────────────────────────────────────────────────────────────────

/**
 * parseDSNEgressAllow parses MCP_DSN_EGRESS_ALLOW. Throws on a bad entry;
 * the caller then allows none (fail closed), as the proxy does.
 */
export function parseDSNEgressAllow(raw) {
  const out = [];
  for (let item of String(raw ?? '').split(',')) {
    item = item.trim();
    if (!item) continue;
    const hp = splitHostPort(item);
    if (hp && item.startsWith('[')) hp.host = `[${hp.host}]`; // keep brackets for the IPv6-only check
    if (!hp || !hostSyntaxOK(hp.host)) throw new Error(`MCP_DSN_EGRESS_ALLOW: ${JSON.stringify(item)} is not host:port`);
    if (!validPort(hp.port)) throw new Error(`MCP_DSN_EGRESS_ALLOW: ${JSON.stringify(item)} has no valid port`);
    const host = canonicalHost(hp.host);
    if (!host || host.startsWith('*')) throw new Error(`MCP_DSN_EGRESS_ALLOW: ${JSON.stringify(item)} must name one host`);
    const why = alwaysDenied(host);
    if (why) throw new Error(`MCP_DSN_EGRESS_ALLOW: ${JSON.stringify(item)}: ${why}`);
    out.push({ host, port: hp.port });
  }
  return out;
}

/**
 * loadEgressRule reads the bridge's egress configuration from env. Returns
 * null when enforcement is off. Configuration errors fail closed and are
 * returned in `errors` for the caller to log.
 */
export function loadEgressRule(env = process.env) {
  if ((env.BRIDGE_DSN_EGRESS || '').trim().toLowerCase() !== 'enforce') return null;
  const errors = [];
  let platform = [];
  try {
    platform = parseDSNEgressAllow(env.MCP_DSN_EGRESS_ALLOW || '');
  } catch (e) {
    errors.push(e.message);
  }
  let tenants = {};
  const rawTenants = (env.MCP_TENANT_EGRESS_ALLOWLIST || '').trim();
  if (rawTenants) {
    try {
      const parsed = JSON.parse(rawTenants);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('not an object');
      tenants = parsed;
    } catch (e) {
      errors.push(`MCP_TENANT_EGRESS_ALLOWLIST: ${e.message}`);
    }
  }
  const namespaces = (env.MCP_TENANT_CLUSTER_NAMESPACES || '').split(',').map((s) => s.trim()).filter(Boolean);
  return { platform, tenants, namespaces: namespaces.length ? namespaces : ['tas-mcp-servers'], errors };
}

// ─── the rule ───────────────────────────────────────────────────────────────

/**
 * checkHost applies the host:port rule. Returns { ok: true, kind } or
 * { ok: false, reason }. The reason names the host, never a credential.
 */
export function checkHost(rule, tenant, rawHost, port) {
  const no = (reason) => ({ ok: false, reason });
  if (!hostSyntaxOK(rawHost)) return no(`${JSON.stringify(String(rawHost))} is not a plain host name or IP address`);
  const host = canonicalHost(rawHost);
  if (!host) return no('no host');
  if (!validPort(port)) return no(`port ${port} is not a valid port`);
  const why = alwaysDenied(host);
  if (why) return no(why);
  if (rule.platform.some((hp) => hp.host === host && hp.port === port)) return { ok: true, kind: 'platform', host };
  const ns = clusterNamespace(host);
  if (ns !== null) {
    if (rule.namespaces.includes(ns)) return { ok: true, kind: 'cluster', host };
    return no(`${host}:${port} is not on MCP_DSN_EGRESS_ALLOW and namespace ${ns} is not allowed`);
  }
  if (isClusterInternal(host)) return no(`${host} is cluster-internal`);
  const addr = parseAddr(host);
  const ip = addr !== null;
  if (!host.includes('.') && !ip) return no(`${host} is a bare name`);
  // A private IP literal is opened only by MCP_DSN_EGRESS_ALLOW (above), the
  // same at store time, use time and dial time.
  if (ip && isPrivateAddr(addr)) return no(`${host} is a private address; only MCP_DSN_EGRESS_ALLOW can open one`);
  const allow = (tenant && Array.isArray(rule.tenants[tenant])) ? rule.tenants[tenant] : [];
  for (const raw of allow) {
    const e = splitAllowEntry(raw);
    if (!e || (e.port !== 0 && e.port !== port)) continue;
    if (e.host.startsWith('*.')) {
      if (!ip && host.endsWith(e.host.slice(1))) return { ok: true, kind: 'tenant', host };
    } else if (e.host === host) {
      return { ok: true, kind: 'tenant', host };
    }
  }
  return no(`${host}:${port} is not on MCP_DSN_EGRESS_ALLOW or this tenant's egress allowlist`);
}

/**
 * checkAddresses applies the dial rule to resolved addresses: every one must
 * pass. Returns '' when they do, else the reason.
 */
export function checkAddresses(kind, addresses) {
  if (!addresses || addresses.length === 0) return 'the host resolved to no address';
  for (const raw of addresses) {
    const a = parseAddr(raw);
    if (!a) return `resolver returned ${raw}, which is not an address`;
    if (matchesAny(a, DIAL_DENIED)) return `the host resolves to ${formatAddr(a)}, which is never allowed`;
    if (kind === 'tenant' && isPrivateAddr(a)) {
      return `the host resolves to ${formatAddr(a)}, a private address; a tenant-listed host must be public`;
    }
  }
  return '';
}

// The ssl_mode values a connection may carry (the proxy's DSNSSLModes), and
// what node-postgres (pg-connection-string 2.x, pinned in the postgres-mcp
// manifest) does with each:
//   - '' / absent: no TLS              -> pinned to the checked address
//   - disable: no TLS                  -> pinned
//   - no-verify: TLS, no verification  -> pinned
//   - allow, prefer, require, verify-ca, verify-full: TLS verified against
//     the host name (pg treats every one of them as verify-full)
//                                      -> the name is kept: pinning would fail
//     the certificate check, and TLS identity refuses a rebound target.
const SSL_MODES = new Set(['', 'disable', 'no-verify', 'allow', 'prefer', 'require', 'verify-ca', 'verify-full']);
const VERIFYING_SSL = new Set(['allow', 'prefer', 'require', 'verify-ca', 'verify-full']);

// The only fields a _connection may carry. Anything else (sslrootcert,
// sslkey, options, hostaddr...) is refused rather than ignored, so a future
// connectionToEnv cannot start honouring one by accident.
const CONNECTION_FIELDS = new Set(['host', 'port', 'username', 'password', 'database', 'protocol', 'ssl_mode']);
// User names and databases are put into a URL: nothing that could start a new
// URL part (@ : / ? # & =) or a parameter.
const IDENT_RE = /^[A-Za-z0-9._-]+$/;
const PROTOCOL_RE = /^[a-z][a-z0-9+]{0,15}$/;
const MAX_PASSWORD = 1024;

/** EgressError is a refusal; its message is safe to return to the caller. */
export class EgressError extends Error {}

/**
 * checkFields validates every field of a _connection (the re-review's HIGH:
 * the host and port were checked while the user name, database or ssl_mode
 * could rewrite the URL the child dials). Returns '' or the reason, which
 * never quotes a value.
 */
export function checkFields(conn) {
  if (!conn || typeof conn !== 'object' || Array.isArray(conn)) return '_connection must be an object';
  for (const k of Object.keys(conn)) {
    if (!CONNECTION_FIELDS.has(k)) return `_connection field ${JSON.stringify(k).slice(0, 40)} is not allowed`;
  }
  if (typeof conn.host !== 'string') return 'host must be a string';
  if (typeof conn.port !== 'number' || !validPort(conn.port)) return 'port must be an integer from 1 to 65535';
  if (typeof conn.username !== 'string' || !IDENT_RE.test(conn.username)) return "username must be letters, digits, '.', '_' or '-'";
  if (typeof conn.database !== 'string' || !IDENT_RE.test(conn.database)) return "database must be letters, digits, '.', '_' or '-'";
  if (conn.password !== undefined && (typeof conn.password !== 'string' || conn.password.length > MAX_PASSWORD)) {
    return 'password must be a string';
  }
  if (conn.protocol !== undefined && (typeof conn.protocol !== 'string' || !PROTOCOL_RE.test(conn.protocol))) {
    return 'protocol is not a protocol name';
  }
  if (conn.ssl_mode !== undefined && (typeof conn.ssl_mode !== 'string' || !SSL_MODES.has(conn.ssl_mode))) {
    return 'ssl_mode must be one of disable, no-verify, allow, prefer, require, verify-ca, verify-full';
  }
  return '';
}

/**
 * guardConnection checks a `_connection` and returns the connection to dial:
 * a copy whose host is the checked address (pinned), or a copy with the name
 * when TLS verification protects it. Throws EgressError when refused. A null
 * or undefined connection (the bridge's own default login) passes through.
 */
export async function guardConnection(rule, conn, tenant, { lookup = dns.lookup } = {}) {
  if (conn === null || conn === undefined) return conn;
  const badField = checkFields(conn);
  if (badField) throw new EgressError(`connection refused by egress policy: ${badField}`);
  const port = conn.port;
  const verdict = checkHost(rule, tenant, conn.host, port);
  if (!verdict.ok) throw new EgressError(`connection refused by egress policy: ${verdict.reason}`);
  let addresses;
  const literal = parseAddr(verdict.host);
  if (literal) {
    addresses = [formatAddr(literal)];
  } else {
    try {
      addresses = (await lookup(verdict.host, { all: true, verbatim: true })).map((r) => r.address);
    } catch {
      throw new EgressError(`connection refused by egress policy: ${verdict.host} did not resolve`);
    }
  }
  const bad = checkAddresses(verdict.kind, addresses);
  if (bad) throw new EgressError(`connection refused by egress policy: ${bad}`);
  if (VERIFYING_SSL.has(conn.ssl_mode || '')) return { ...conn, host: verdict.host };
  const a = parseAddr(addresses[0]);
  return { ...conn, host: formatAddr(a) };
}
