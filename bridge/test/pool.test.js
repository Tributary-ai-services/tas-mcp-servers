// Bridge child-process isolation (security review, 2026-11-21).
// Run: npm test   (node --test, no dependencies)

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';

import { MCPProcessManager, connectionKey, slotKey, tenantOf, DEFAULT_KEY } from '../src/pool.js';

const TENANT_A = '6f1d2e3f-0000-4000-8000-00000000000a';
const TENANT_B = '6f1d2e3f-0000-4000-8000-00000000000b';

// fakeSpawn records each child and its env, and answers every JSON-RPC
// request with a result naming the child, so a test can tell which child
// served a call.
function fakeSpawn() {
  const children = [];
  const spawn = (cmd, args, opts) => {
    const child = new EventEmitter();
    child.n = children.length;
    child.env = opts.env;
    child.killed = false;
    child.stdout = new EventEmitter();
    child.stdin = {
      write(line) {
        const msg = JSON.parse(line);
        if (msg.id != null) {
          setImmediate(() => child.stdout.emit('data', JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { child: child.n } }) + '\n'));
        }
      },
    };
    child.kill = () => {
      child.killed = true;
      setImmediate(() => child.emit('exit', null));
    };
    children.push(child);
    return child;
  };
  return { spawn, children };
}

const conn = (password, over = {}) => ({ host: 'db.vendor.example', port: 5432, database: 'sales', username: 'alice', password, ...over });

function manager(spawn, maxChildren) {
  return new MCPProcessManager({ command: 'mcp-server', serverType: 'postgres', spawn, maxChildren, env: {} });
}

async function servedBy(m, c, tenant) {
  const r = await m.sendRequest(c, 'tools/call', { name: 'q', arguments: {} }, tenant);
  return r.result.child;
}

test('a wrong password never reuses another caller\'s child', async () => {
  const { spawn, children } = fakeSpawn();
  const m = manager(spawn);
  const a = await servedBy(m, conn('A-real-password'), TENANT_A);
  // B, or an unattributed caller, with A's host/port/db/user and a guessed password.
  const b = await servedBy(m, conn('guess'), TENANT_B);
  const u = await servedBy(m, conn('guess'), '');
  assert.notEqual(b, a);
  assert.notEqual(u, a);
  assert.match(children[b].env.DATABASE_URL, /:guess@/);
  assert.match(children[a].env.DATABASE_URL, /:A-real-password@/);
});

test('the same connection under another tenant gets its own child', async () => {
  const { spawn } = fakeSpawn();
  const m = manager(spawn);
  const a = await servedBy(m, conn('shared-pw'), TENANT_A);
  const b = await servedBy(m, conn('shared-pw'), TENANT_B);
  const u = await servedBy(m, conn('shared-pw'), '');
  assert.equal(new Set([a, b, u]).size, 3);
});

test('the same tenant and connection reuse one child', async () => {
  const { spawn, children } = fakeSpawn();
  const m = manager(spawn);
  const first = await servedBy(m, conn('pw-1'), TENANT_A);
  const again = await servedBy(m, conn('pw-1'), TENANT_A);
  assert.equal(first, again);
  assert.equal(children.length, 1);
});

test('a rotation kills the child holding the old secret', async () => {
  const { spawn, children } = fakeSpawn();
  const m = manager(spawn);
  const old = await servedBy(m, conn('old-secret'), TENANT_A);
  const fresh = await servedBy(m, conn('new-secret'), TENANT_A);
  assert.notEqual(fresh, old);
  assert.equal(children[old].killed, true);
  assert.equal(children[fresh].killed, false);
  // The old secret no longer has a live child: presenting it starts a new
  // process (which the database then authenticates), it is not served by the
  // retired one, and that in turn retires the new-secret child.
  const replay = await servedBy(m, conn('old-secret'), TENANT_A);
  assert.notEqual(replay, old);
  // A rotation for one tenant leaves another tenant's child alone.
  const b = await servedBy(m, conn('b-secret'), TENANT_B);
  await servedBy(m, conn('b-secret-2'), TENANT_A);
  assert.equal(children[b].killed, false);
});

test('keys hold no plaintext and differ by password and tenant', () => {
  const k = connectionKey(conn('Pw-plaintext'), TENANT_A);
  assert.match(k, /^[0-9a-f]{64}$/);
  assert.ok(!k.includes('Pw-plaintext') && !k.includes('alice'));
  assert.notEqual(k, connectionKey(conn('Pw-other'), TENANT_A));
  assert.notEqual(k, connectionKey(conn('Pw-plaintext'), TENANT_B));
  // Key order in the object does not matter.
  const c = conn('x');
  const reordered = { password: c.password, username: c.username, database: c.database, port: c.port, host: c.host };
  assert.equal(connectionKey(c, TENANT_A), connectionKey(reordered, TENANT_A));
  assert.equal(slotKey(conn('one'), TENANT_A), slotKey(conn('two'), TENANT_A));
  assert.notEqual(slotKey(conn('one'), TENANT_A), slotKey(conn('one'), TENANT_B));
  assert.equal(connectionKey(null, TENANT_A), DEFAULT_KEY);
});

test('the pool is bounded (LRU eviction, the default child kept)', async () => {
  const { spawn, children } = fakeSpawn();
  const m = manager(spawn, 3);
  await m.sendRequest(null, 'tools/list', {});
  for (let i = 0; i < 5; i++) await servedBy(m, conn(`pw-${i}`, { username: `u${i}` }), TENANT_A);
  assert.ok(m.processes.size <= 3, `pool size ${m.processes.size}`);
  assert.ok(m.processes.has(DEFAULT_KEY), 'default child evicted');
  assert.equal(children[0].killed, false);
});

test('tenantOf reads only a well-formed proxy tenant header', () => {
  assert.equal(tenantOf({ 'tas-tenant-id': TENANT_A.toUpperCase() }), TENANT_A);
  assert.equal(tenantOf({ 'tas-tenant-id': 'x; drop' }), '');
  assert.equal(tenantOf({}), '');
});
