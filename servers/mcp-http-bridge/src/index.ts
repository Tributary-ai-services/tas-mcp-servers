#!/usr/bin/env node
/**
 * TAS MCP HTTP Bridge
 *
 * Generic stdio-to-HTTP bridge for MCP servers. Wraps any stdio MCP server
 * with TAS-compatible HTTP endpoints following the napkin-mcp pattern:
 *   GET  /health          - Health check
 *   GET  /mcp/tools/list  - List available tools
 *   POST /mcp/tools/call  - Execute a tool
 *
 * Environment variables:
 *   MCP_COMMAND      - Command to run the stdio MCP server (required)
 *   MCP_ARGS         - Space-separated arguments for the command
 *   MCP_ENV_PREFIX   - Prefix for env vars to pass through to child process
 *   BRIDGE_PORT      - HTTP port to listen on (default: 8000)
 *   BRIDGE_NAME      - Server name for health endpoint (default: derived from MCP_COMMAND)
 *   BRIDGE_VERSION   - Server version for health endpoint (default: 1.0.0)
 *
 * Usage:
 *   MCP_COMMAND="npx" MCP_ARGS="-y @upstash/context7-mcp" BRIDGE_PORT=8000 node dist/index.js
 *   MCP_COMMAND="npx" MCP_ARGS="-y @modelcontextprotocol/server-sequential-thinking" node dist/index.js
 */
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import http from "http";
import { spawn, ChildProcess } from "child_process";

const BRIDGE_PORT = parseInt(process.env.BRIDGE_PORT || "8000", 10);
const MCP_COMMAND = process.env.MCP_COMMAND;
const MCP_ARGS = process.env.MCP_ARGS?.split(" ").filter(Boolean) || [];
const BRIDGE_NAME = process.env.BRIDGE_NAME || MCP_COMMAND || "mcp-http-bridge";
const BRIDGE_VERSION = process.env.BRIDGE_VERSION || "1.0.0";

let client: Client | null = null;
let connected = false;
let childProcess: ChildProcess | null = null;
let reconnecting = false;

// Build environment for child process - pass through all env vars
function getChildEnv(): Record<string, string> {
  const env: Record<string, string> = {};
  for (const [key, value] of Object.entries(process.env)) {
    if (value !== undefined) {
      env[key] = value;
    }
  }
  return env;
}

async function connectToStdioServer(): Promise<void> {
  if (!MCP_COMMAND) {
    throw new Error("MCP_COMMAND environment variable is required");
  }

  console.error(`[bridge] Starting stdio MCP server: ${MCP_COMMAND} ${MCP_ARGS.join(" ")}`);

  const transport = new StdioClientTransport({
    command: MCP_COMMAND,
    args: MCP_ARGS,
    env: getChildEnv(),
  });

  client = new Client(
    { name: "tas-mcp-bridge", version: BRIDGE_VERSION },
    { capabilities: {} }
  );

  await client.connect(transport);
  connected = true;
  console.error(`[bridge] Connected to stdio MCP server`);
}

async function ensureConnected(): Promise<Client> {
  if (!client || !connected) {
    if (!reconnecting) {
      reconnecting = true;
      try {
        await connectToStdioServer();
      } finally {
        reconnecting = false;
      }
    }
    // Wait for reconnection
    const deadline = Date.now() + 30000;
    while (!connected && Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 100));
    }
  }
  if (!client || !connected) {
    throw new Error("Not connected to MCP server");
  }
  return client;
}

// Read HTTP request body
function readBody(req: http.IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    req.on("data", (chunk: Buffer) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks).toString()));
    req.on("error", reject);
  });
}

function startHttpServer(): void {
  const httpServer = http.createServer(async (req, res) => {
    // CORS headers
    res.setHeader("Access-Control-Allow-Origin", "*");
    res.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
    res.setHeader("Access-Control-Allow-Headers", "Content-Type, X-Space-ID, X-Space-Type, X-Tenant-ID");

    if (req.method === "OPTIONS") {
      res.writeHead(204);
      res.end();
      return;
    }

    // Health check
    if (req.url === "/health" && req.method === "GET") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          status: connected ? "healthy" : "degraded",
          service: BRIDGE_NAME,
          version: BRIDGE_VERSION,
          bridge: "tas-mcp-http-bridge",
          connected,
          timestamp: new Date().toISOString(),
        })
      );
      return;
    }

    // List tools
    if (req.url === "/mcp/tools/list" && req.method === "GET") {
      try {
        const c = await ensureConnected();
        const result = await c.listTools();
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ tools: result.tools }));
      } catch (error: any) {
        res.writeHead(503, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: error.message, tools: [] }));
      }
      return;
    }

    // Call tool
    if (req.url === "/mcp/tools/call" && req.method === "POST") {
      try {
        const body = await readBody(req);
        const { name, arguments: toolArgs } = JSON.parse(body);

        if (!name) {
          res.writeHead(400, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ error: "Missing 'name' field", isError: true }));
          return;
        }

        const c = await ensureConnected();
        const result = await c.callTool({ name, arguments: toolArgs || {} });

        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify(result));
      } catch (error: any) {
        res.writeHead(500, { "Content-Type": "application/json" });
        res.end(
          JSON.stringify({
            content: [{ type: "text", text: `Error: ${error.message}` }],
            isError: true,
          })
        );
      }
      return;
    }

    // 404 for everything else
    res.writeHead(404, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ error: "Not found" }));
  });

  httpServer.listen(BRIDGE_PORT, () => {
    console.error(`[bridge] HTTP server listening on port ${BRIDGE_PORT}`);
  });
}

async function main() {
  if (!MCP_COMMAND) {
    console.error("Error: MCP_COMMAND environment variable is required");
    console.error("Usage: MCP_COMMAND=npx MCP_ARGS='-y @upstash/context7-mcp' node dist/index.js");
    process.exit(1);
  }

  // Start HTTP server first (so health endpoint is available during startup)
  startHttpServer();

  // Connect to the stdio MCP server
  try {
    await connectToStdioServer();
  } catch (error: any) {
    console.error(`[bridge] Failed to connect to stdio server: ${error.message}`);
    console.error("[bridge] HTTP server running in degraded mode, will retry on next request");
  }
}

// Handle graceful shutdown
process.on("SIGTERM", () => {
  console.error("[bridge] Received SIGTERM, shutting down");
  if (childProcess) {
    childProcess.kill("SIGTERM");
  }
  process.exit(0);
});

process.on("SIGINT", () => {
  console.error("[bridge] Received SIGINT, shutting down");
  if (childProcess) {
    childProcess.kill("SIGTERM");
  }
  process.exit(0);
});

main().catch((error) => {
  console.error("[bridge] Fatal error:", error);
  process.exit(1);
});
