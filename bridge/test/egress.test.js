// DSN egress at dial time (MCP-19). Run: npm test (node --test, no dependencies)

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';

import {
  canonicalHost, checkHost, checkAddresses, guardConnection, loadEgressRule, parseDSNEgressAllow, EgressError,
} from '../src/egress.js';
import { handleCallTool } from '../src/server.js';

// Same file, same hash, as tas-mcp internal/federation/testdata and
// aiqg-dashboard-be internal/mcpconfig/testdata. Change all three together.
const VECTORS_SHA256 = '2a2c2469c79b6d36a82fa85836921e1206f7788066bb82709ff9f08c15f4f778';
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
  assert.equal(v6.host, '[2001:db8::20]');
  for (const ssl of ['require', 'verify-ca', 'verify-full']) {
    const kept = await guardConnection(rule, { ...base, ssl_mode: ssl }, tenant, { lookup });
    assert.equal(kept.host, base.host, ssl);
  }
  // disable / prefer / no-verify: pinned (prefer may fall back to plaintext).
  for (const ssl of ['disable', 'prefer', 'no-verify', '']) {
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
