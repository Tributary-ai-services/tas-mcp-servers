#!/usr/bin/env node
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
  ListResourcesRequestSchema,
  ReadResourceRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import http from "http";
import {
  GenerateVisualSchema,
  CheckVisualStatusSchema,
  DownloadVisualSchema,
  ListVisualsSchema,
  CreateNapkinVisualCRSchema,
  GeneratedVisualFile,
  GenerateVisualResult,
} from "./types";
import { NapkinClient } from "./napkin-client";
import { MinioClient } from "./minio-client";

const HEALTH_PORT = parseInt(process.env.HEALTH_PORT || "8087", 10);

// Create clients
const napkinClient = new NapkinClient();
const minioClient = new MinioClient();

// Content type mapping
function getContentType(format: string): string {
  switch (format) {
    case "svg":
      return "image/svg+xml";
    case "png":
      return "image/png";
    case "ppt":
      return "application/vnd.ms-powerpoint";
    default:
      return "application/octet-stream";
  }
}

// Create and configure the MCP server
const server = new Server(
  {
    name: "napkin-mcp",
    version: "1.0.0",
  },
  {
    capabilities: {
      tools: {},
      resources: {},
    },
  }
);

// List available tools
server.setRequestHandler(ListToolsRequestSchema, async () => {
  return {
    tools: [
      {
        name: "generate_visual",
        description:
          "Generate a visual from text using Napkin AI. Submits text content, waits for processing, downloads the result, and stores it permanently in MinIO. Returns the MinIO URL(s) for the generated visual(s).",
        inputSchema: {
          type: "object",
          properties: {
            content: {
              type: "string",
              description: "Text content to visualize (1-50000 characters)",
              minLength: 1,
              maxLength: 50000,
            },
            format: {
              type: "string",
              enum: ["svg", "png", "ppt"],
              description: "Output format",
              default: "svg",
            },
            style_id: {
              type: "string",
              description: "Napkin AI style identifier",
            },
            color_mode: {
              type: "string",
              enum: ["light", "dark", "both"],
              description: "Color mode for the visual",
              default: "light",
            },
            language: {
              type: "string",
              description: "Language code (BCP 47)",
              default: "en",
            },
            variations: {
              type: "number",
              description: "Number of variations to generate (1-5)",
              default: 1,
              minimum: 1,
              maximum: 5,
            },
            context: {
              type: "string",
              description: "Additional context for generation",
            },
          },
          required: ["content"],
        },
      },
      {
        name: "check_visual_status",
        description: "Check the status of a pending Napkin AI visual generation request",
        inputSchema: {
          type: "object",
          properties: {
            request_id: {
              type: "string",
              description: "Napkin AI request ID",
            },
          },
          required: ["request_id"],
        },
      },
      {
        name: "download_visual",
        description: "Download a generated visual from MinIO storage",
        inputSchema: {
          type: "object",
          properties: {
            minio_key: {
              type: "string",
              description: "MinIO object key for the visual",
            },
            bucket: {
              type: "string",
              description: "MinIO bucket name",
              default: "napkin-visuals",
            },
          },
          required: ["minio_key"],
        },
      },
      {
        name: "list_styles",
        description: "List available Napkin AI visual styles",
        inputSchema: {
          type: "object",
          properties: {},
        },
      },
      {
        name: "list_visuals",
        description: "List generated visuals stored in MinIO",
        inputSchema: {
          type: "object",
          properties: {
            prefix: {
              type: "string",
              description: "Object key prefix to filter by",
            },
            bucket: {
              type: "string",
              description: "MinIO bucket name",
              default: "napkin-visuals",
            },
            limit: {
              type: "number",
              description: "Maximum number of results (1-100)",
              default: 20,
              minimum: 1,
              maximum: 100,
            },
          },
        },
      },
      {
        name: "create_napkin_visual_cr",
        description:
          "Create a NapkinVisual custom resource in Kubernetes for operator-managed visual generation",
        inputSchema: {
          type: "object",
          properties: {
            name: {
              type: "string",
              description: "Name for the NapkinVisual custom resource",
            },
            namespace: {
              type: "string",
              description: "Kubernetes namespace",
              default: "tas-mcp-servers",
            },
            content: {
              type: "string",
              description: "Text content to visualize",
            },
            format: {
              type: "string",
              enum: ["svg", "png", "ppt"],
              description: "Output format",
              default: "svg",
            },
            style_id: {
              type: "string",
              description: "Napkin AI style identifier",
            },
            color_mode: {
              type: "string",
              enum: ["light", "dark", "both"],
              description: "Color mode",
              default: "light",
            },
          },
          required: ["name", "content"],
        },
      },
    ],
  };
});

// Handle tool calls
server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request.params;

  try {
    switch (name) {
      case "generate_visual": {
        const parsed = GenerateVisualSchema.parse(args);
        const startTime = Date.now();

        // Submit visual generation request
        const submission = await napkinClient.submitVisual(parsed);
        const requestId = submission.id;

        // Poll until complete
        const completed = await napkinClient.waitForCompletion(requestId);

        if (!completed.files || completed.files.length === 0) {
          throw new Error("No files generated");
        }

        // Download each file and upload to MinIO
        const generatedFiles: GeneratedVisualFile[] = [];
        for (const file of completed.files) {
          const data = await napkinClient.downloadFile(file.url);
          const key = `visuals/${requestId}/${file.index}.${file.format}`;
          const contentType = getContentType(file.format);
          const uploaded = await minioClient.upload(key, data, contentType);

          generatedFiles.push({
            index: file.index,
            format: file.format,
            color_mode: file.color_mode,
            napkin_url: file.url,
            minio_key: uploaded.key,
            minio_url: uploaded.url,
            size_bytes: uploaded.size_bytes,
          });
        }

        const result: GenerateVisualResult = {
          request_id: requestId,
          status: "completed",
          files: generatedFiles,
          total_time_ms: Date.now() - startTime,
        };

        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(result, null, 2),
            },
          ],
        };
      }

      case "check_visual_status": {
        const parsed = CheckVisualStatusSchema.parse(args);
        const status = await napkinClient.getVisualStatus(parsed.request_id);
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(status, null, 2),
            },
          ],
        };
      }

      case "download_visual": {
        const parsed = DownloadVisualSchema.parse(args);
        const data = await minioClient.download(parsed.minio_key, parsed.bucket);
        const base64 = data.toString("base64");
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(
                {
                  key: parsed.minio_key,
                  bucket: parsed.bucket,
                  size_bytes: data.length,
                  data_base64: base64,
                },
                null,
                2
              ),
            },
          ],
        };
      }

      case "list_styles": {
        const styles = await napkinClient.listStyles();
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(styles, null, 2),
            },
          ],
        };
      }

      case "list_visuals": {
        const parsed = ListVisualsSchema.parse(args);
        const objects = await minioClient.list(parsed.prefix, parsed.bucket, parsed.limit);
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(objects, null, 2),
            },
          ],
        };
      }

      case "create_napkin_visual_cr": {
        const parsed = CreateNapkinVisualCRSchema.parse(args);
        const cr = {
          apiVersion: "napkin.tas.ai/v1",
          kind: "NapkinVisual",
          metadata: {
            name: parsed.name,
            namespace: parsed.namespace,
          },
          spec: {
            content: parsed.content,
            format: parsed.format,
            style: {
              styleId: parsed.style_id || "",
              colorMode: parsed.color_mode,
            },
          },
        };
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(
                {
                  message:
                    "NapkinVisual CR manifest generated. Apply with: kubectl apply -f <file>",
                  manifest: cr,
                },
                null,
                2
              ),
            },
          ],
        };
      }

      default:
        throw new Error(`Unknown tool: ${name}`);
    }
  } catch (error: any) {
    return {
      content: [
        {
          type: "text",
          text: `Error: ${error.message}`,
        },
      ],
      isError: true,
    };
  }
});

// List available resources
server.setRequestHandler(ListResourcesRequestSchema, async () => {
  return {
    resources: [
      {
        uri: "napkin://styles",
        name: "Napkin AI Styles",
        description: "Available visual styles for Napkin AI generation",
        mimeType: "application/json",
      },
      {
        uri: "napkin://visuals/recent",
        name: "Recent Visuals",
        description: "Recently generated visuals stored in MinIO",
        mimeType: "application/json",
      },
    ],
  };
});

// Read resources
server.setRequestHandler(ReadResourceRequestSchema, async (request) => {
  const { uri } = request.params;

  switch (uri) {
    case "napkin://styles": {
      try {
        const styles = await napkinClient.listStyles();
        return {
          contents: [
            {
              uri,
              mimeType: "application/json",
              text: JSON.stringify(styles, null, 2),
            },
          ],
        };
      } catch (error: any) {
        return {
          contents: [
            {
              uri,
              mimeType: "application/json",
              text: JSON.stringify({ error: error.message }),
            },
          ],
        };
      }
    }

    case "napkin://visuals/recent": {
      try {
        const visuals = await minioClient.list("visuals/", undefined, 20);
        return {
          contents: [
            {
              uri,
              mimeType: "application/json",
              text: JSON.stringify(visuals, null, 2),
            },
          ],
        };
      } catch (error: any) {
        return {
          contents: [
            {
              uri,
              mimeType: "application/json",
              text: JSON.stringify({ error: error.message }),
            },
          ],
        };
      }
    }

    default:
      throw new Error(`Unknown resource: ${uri}`);
  }
});

// Health check HTTP server
function startHealthServer(): void {
  const healthServer = http.createServer((req, res) => {
    if (req.url === "/health" && req.method === "GET") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          status: "healthy",
          service: "napkin-mcp",
          version: "1.0.0",
          timestamp: new Date().toISOString(),
        })
      );
    } else {
      res.writeHead(404);
      res.end();
    }
  });

  healthServer.listen(HEALTH_PORT, () => {
    console.error(`Health check server listening on port ${HEALTH_PORT}`);
  });
}

// Main entry point
async function main() {
  startHealthServer();

  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("Napkin MCP server running on stdio");
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exit(1);
});
