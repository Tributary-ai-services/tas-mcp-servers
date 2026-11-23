// DSN egress at dial time (MCP-19). Run: npm test (node --test, no dependencies)

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';

import {
  canonicalHost, checkHost, checkAddresses, guardConnection, loadEgressRule, parseDSNEgressAllow, EgressError,
} from '../src/egress.js';
import { handleCallTool } from '../src/server.js';
import { connectionToEnv, postgresURL } from '../src/pool.js';

// Same file, same hash, as tas-mcp internal/federation/testdata and
// aiqg-dashboard-be internal/mcpconfig/testdata. Change all three together.
const VECTORS_SHA256 = '05b7ea3cc0d6cfc2fa5abedb7b86c5a095ced6840f82fbe43c7bf1508868ec16';
const raw = readFileSync(new URL('./dsn_egress_vectors.json', import.meta.url));
const V = JSON.parse(raw);

const ruleFromVectors = () => loadEgressRule({
  BRIDGE_DSN_EGRESS: 'enforce',
  MCP_DSN_EGRESS_ALLOW: V.config.dsn_egress_allow,
  MCP_TENANT_EGRESS_ALLOWLIST: JSON.stringify(V.config.tenant_egress_allowlist),
  MCP_TENANT_CLUSTER_NAMESPACES: V.config.cluster_namespaces.join(','),
});

const fakeLookup = (answers) => async () => answers.map((address) => ({ address, family: address.includes(':') ? 6 : 4 }));

test('vectors file is the shared one', () => {
  assert.equal(createHash('sha256').update(raw).digest('hex'), VECTORS_SHA256,
    'dsn_egress_vectors.json changed: copy it to tas-mcp and aiqg-dashboard-be and update the constant in all three');
  assert.ok(V.cases.length >= 50);
});

test('host:port rule agrees with the proxy and AIQG (shared vectors)', () => {
  const rule = ruleFromVectors();
  assert.deepEqual(rule.errors, []);
  let n = 0;
  for (const c of V.cases) {
    if (c.registry === false) continue; // proxy-only (tenant registry off)
    n++;
    const got = checkHost(rule, V.config.tenants[c.tenant], c.host, c.port);
    assert.equal(got.ok, c.allow, `${c.name}: ${c.host}:${c.port} -> ${JSON.stringify(got)}`);
  }
  assert.ok(n >= 50, `only ${n} vectors ran`);
});

test('dial rule over resolved addresses (DNS rebinding vectors)', async () => {
  const rule = ruleFromVectors();
  let n = 0;
  for (const c of V.cases) {
    if (!('resolve' in c)) continue;
    n++;
    const conn = { host: c.host, port: c.port, username: 'u', password: 'pw-12345678', database: 'db' };
    let ok = true;
    try {
      await guardConnection(rule, conn, V.config.tenants[c.tenant], { lookup: fakeLookup(c.resolve) });
    } catch (e) {
      assert.ok(e instanceof EgressError, `${c.name}: ${e}`);
      assert.ok(!e.message.includes('pw-12345678'));
      ok = false;
    }
    assert.equal(ok, c.dial_allow, `${c.name}: resolve ${JSON.stringify(c.resolve)}`);
  }
  assert.ok(n >= 10, `only ${n} rebinding vectors ran`);
});

test('the checked address is the one dialed (pinned), unless TLS verifies the name', async () => {
  const rule = ruleFromVectors();
  const tenant = V.config.tenants.a;
  const base = { host: 'postgres-shared.tas-shared.svc.cluster.local', port: 5432, username: 'u', password: 'p', database: 'db' };
  let calls = 0;
  const lookup = async () => { calls++; return [{ address: '10.43.109.5', family: 4 }]; };
  const pinned = await guardConnection(rule, base, tenant, { lookup });
  assert.equal(pinned.host, '10.43.109.5');
  assert.equal(base.host, 'postgres-shared.tas-shared.svc.cluster.local', 'original not mutated');
  assert.equal(calls, 1);
  const v6 = await guardConnection(rule, { ...base, host: 'pg.vendor.example' }, tenant,
    { lookup: fakeLookup(['2001:db8::20']) });
  assert.equal(v6.host, '2001:db8::20');
  assert.equal(new URL(postgresURL(v6)).hostname, '[2001:db8::20]');
  // node-postgres verifies the certificate for every TLS mode but no-verify
  // (allow and prefer included), so those keep the name (LOW-B).
  for (const ssl of ['allow', 'prefer', 'require', 'verify-ca', 'verify-full']) {
    const kept = await guardConnection(rule, { ...base, ssl_mode: ssl }, tenant, { lookup });
    assert.equal(kept.host, base.host, ssl);
  }
  for (const ssl of ['disable', 'no-verify', '']) {
    const p = await guardConnection(rule, { ...base, ssl_mode: ssl }, tenant, { lookup });
    assert.equal(p.host, '10.43.109.5', ssl);
  }
});

test('a lookup failure refuses', async () => {
  const rule = ruleFromVectors();
  await assert.rejects(guardConnection(rule, { host: 'pg.vendor.example', port: 5432 }, V.config.tenants.a,
    { lookup: async () => { throw new Error('ENOTFOUND'); } }), EgressError);
});

test('refused call never starts a child and never echoes the password', async () => {
  const rule = ruleFromVectors();
  const body = JSON.stringify({ name: 'query', arguments: { sql: 'select 1', _connection: {
    host: 'redis-shared.tas-shared.svc.cluster.local', port: 6379, username: 'u', password: 'pw-12345678', database: '0' } } });
  const res = await handleCallTool(body, V.config.tenants.a, { rule, lookup: fakeLookup(['10.43.0.9']) });
  assert.equal(res.isError, true);
  assert.match(res.content[0].text, /egress policy/);
  assert.ok(!res.content[0].text.includes('pw-12345678'));
  // Direct callers (no tenant header) get the platform list and namespaces only.
  const direct = JSON.stringify({ name: 'query', arguments: { _connection: { host: 'pg.vendor.example', port: 5432, password: 'x' } } });
  assert.equal((await handleCallTool(direct, '', { rule, lookup: fakeLookup(['198.51.100.20']) })).isError, true);
});

test('enforcement is off unless BRIDGE_DSN_EGRESS=enforce; config errors fail closed', () => {
  assert.equal(loadEgressRule({}), null);
  assert.equal(loadEgressRule({ BRIDGE_DSN_EGRESS: 'on' }), null);
  const bad = loadEgressRule({ BRIDGE_DSN_EGRESS: 'enforce', MCP_DSN_EGRESS_ALLOW: 'ok.example:5432,127.0.0.1:5432',
    MCP_TENANT_EGRESS_ALLOWLIST: '{"t":' });
  assert.equal(bad.platform.length, 0);
  assert.deepEqual(bad.tenants, {});
  assert.equal(bad.errors.length, 2);
  assert.deepEqual(bad.namespaces, ['tas-mcp-servers']);
  const none = loadEgressRule({ BRIDGE_DSN_EGRESS: 'enforce' });
  assert.equal(checkHost(none, '', 'postgres-shared.tas-shared.svc.cluster.local', 5432).ok, false, 'default is deny');
});

test('parseDSNEgressAllow mirrors the proxy', () => {
  const got = parseDSNEgressAllow(' postgres-shared.tas-shared.svc:5432 , [2001:db8::5]:5432,,');
  assert.deepEqual(got, [{ host: 'postgres-shared.tas-shared.svc.cluster.local', port: 5432 }, { host: '2001:db8::5', port: 5432 }]);
  for (const bad of ['postgres-shared.tas-shared.svc.cluster.local', 'x.example:0', 'x.example:http',
    '*.tas-shared.svc.cluster.local:5432', '127.0.0.1:5432', 'localhost:5432', '169.254.169.254:80', '[::1]:5432', '127.1:5432']) {
    assert.throws(() => parseDSNEgressAllow(bad), undefined, bad);
  }
  assert.equal(canonicalHost('[::ffff:7f00:1]'), '127.0.0.1');
});

test('dial rule refuses 0.0.0.0/8 and private addresses for tenant hosts only', () => {
  assert.notEqual(checkAddresses('platform', ['0.1.2.3']), '');
  assert.equal(checkAddresses('platform', ['10.43.109.5']), '');
  assert.equal(checkAddresses('cluster', ['fd12::5']), '');
  assert.notEqual(checkAddresses('tenant', ['fd12::5']), '');
  assert.notEqual(checkAddresses('tenant', ['172.20.0.1']), '');
  assert.equal(checkAddresses('tenant', ['8.8.8.8', '2001:4860::8888']), '');
});

// The re-review's HIGH: every field of a _connection, not only host and port.
// Each allowed case must yield a DATABASE_URL that names exactly the checked
// host, the port and the database, with no query but sslmode.
test('field injection: shared connection vectors', async () => {
  const rule = ruleFromVectors();
  assert.ok(V.connection_cases.length >= 25);
  for (const c of V.connection_cases) {
    let dialed = null;
    try {
      dialed = await guardConnection(rule, c.connection, V.config.tenants.a, { lookup: fakeLookup(c.resolve) });
    } catch (e) {
      assert.ok(e instanceof EgressError, `${c.name}: ${e}`);
      assert.ok(!e.message.includes('pw-12345678'), `${c.name}: refusal quotes the password`);
    }
    assert.equal(dialed !== null, c.allow, `${c.name}: allowed=${dialed !== null}`);
    if (!dialed) continue;
    const u = new URL(connectionToEnv(dialed, 'postgres').DATABASE_URL);
    const verifying = ['allow', 'prefer', 'require', 'verify-ca', 'verify-full'].includes(c.connection.ssl_mode);
    assert.equal(u.hostname, verifying ? c.connection.host : c.resolve[0], `${c.name}: host`);
    assert.equal(u.port, String(c.connection.port), `${c.name}: port`);
    assert.equal(decodeURIComponent(u.pathname), `/${c.connection.database}`, `${c.name}: database`);
    assert.equal(decodeURIComponent(u.username), c.connection.username, `${c.name}: username`);
    assert.equal(decodeURIComponent(u.password), c.connection.password, `${c.name}: password round trip`);
    assert.deepEqual([...u.searchParams.keys()].filter((k) => k !== 'sslmode'), [], `${c.name}: query`);
    assert.equal(u.hash, '', `${c.name}: fragment`);
  }
});

test('postgresURL refuses a host or port the URL setters would silently drop', () => {
  for (const host of ['a.b/c', 'a.b?x', 'a.b#x', 'u@a.b', 'a b', '']) {
    assert.throws(() => postgresURL({ host, port: 5432, username: 'u', password: 'p', database: 'd' }), undefined, host);
  }
  for (const port of ['5432x', 0, 70000, 5432.5]) {
    assert.throws(() => postgresURL({ host: 'a.b', port, username: 'u', password: 'p', database: 'd' }), undefined, String(port));
  }
});

test('a non-object _connection is refused, not passed to the pool', async () => {
  const rule = ruleFromVectors();
  for (const conn of ['postgres://x', 7, [], true]) {
    await assert.rejects(guardConnection(rule, conn, V.config.tenants.a, { lookup: fakeLookup(['10.43.109.5']) }), EgressError);
  }
  assert.equal(await guardConnection(rule, null, ''), null, 'the default login still passes');
});

// Second line, and the only one with enforcement off: connectionToEnv encodes
// every part, so a field cannot rewrite the host, port, database or query.
test('connectionToEnv: no field spills into another URL part', () => {
  const env = connectionToEnv({ host: 'postgres-shared.tas-shared.svc.cluster.local', port: 5432,
    username: 'u:x@redis-shared.tas-shared.svc.cluster.local:6379/d#', password: 'p?host=10.0.0.9',
    database: 'd?host=169.254.169.254&port=80', ssl_mode: 'disable&host=10.0.0.9' }, 'postgres');
  const u = new URL(env.DATABASE_URL);
  assert.equal(u.hostname, 'postgres-shared.tas-shared.svc.cluster.local');
  assert.equal(u.port, '5432');
  assert.equal(decodeURIComponent(u.pathname), '/d?host=169.254.169.254&port=80');
  assert.deepEqual([...u.searchParams.keys()], ['sslmode']);
  assert.equal(u.searchParams.get('sslmode'), 'disable&host=10.0.0.9');
  assert.equal(decodeURIComponent(u.username), 'u:x@redis-shared.tas-shared.svc.cluster.local:6379/d#');
  assert.equal(u.hash, '');
});
