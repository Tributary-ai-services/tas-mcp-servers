#!/usr/bin/env node
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
  ListResourcesRequestSchema,
  ReadResourceRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import axios, { AxiosInstance } from "axios";

// Environment configuration
const config = {
  deeplakeUrl: process.env.DEEPLAKE_URL || "http://localhost:8000",
  audimodalUrl: process.env.AUDIMODAL_URL || "http://localhost:8084",
  aetherUrl: process.env.AETHER_URL || "http://localhost:8080",
  apiKey: process.env.TAS_API_KEY || "",
};

// Tool schemas
const ListNotebookDocumentsSchema = z.object({
  notebook_id: z.string().describe("The ID of the notebook to list documents from"),
  include_metadata: z.boolean().optional().default(true).describe("Include document metadata"),
});

const GetDocumentContentSchema = z.object({
  document_id: z.string().describe("The ID of the document to retrieve"),
  format: z.enum(["text", "chunks"]).optional().default("text").describe("Output format"),
});

const SearchDocumentsSchema = z.object({
  query: z.string().describe("The search query"),
  notebook_id: z.string().optional().describe("Filter by notebook ID"),
  top_k: z.number().optional().default(10).describe("Number of results to return"),
});

const GetDocumentSummarySchema = z.object({
  document_id: z.string().describe("The ID of the document to summarize"),
});

// API client for TAS services
class TASClient {
  private deeplakeClient: AxiosInstance;
  private audimodalClient: AxiosInstance;
  private aetherClient: AxiosInstance;

  constructor() {
    const commonHeaders = {
      "Content-Type": "application/json",
      ...(config.apiKey ? { Authorization: `Bearer ${config.apiKey}` } : {}),
    };

    this.deeplakeClient = axios.create({
      baseURL: config.deeplakeUrl,
      headers: commonHeaders,
      timeout: 30000,
    });

    this.audimodalClient = axios.create({
      baseURL: config.audimodalUrl,
      headers: commonHeaders,
      timeout: 30000,
    });

    this.aetherClient = axios.create({
      baseURL: config.aetherUrl,
      headers: commonHeaders,
      timeout: 30000,
    });
  }

  async listNotebookDocuments(notebookId: string, tenantId: string, includeMetadata: boolean) {
    const response = await this.aetherClient.get(
      `/api/v1/internal/notebooks/${notebookId}/documents`,
      {
        params: { include_metadata: includeMetadata },
        headers: { "X-Tenant-ID": tenantId },
      }
    );
    return response.data;
  }

  async getDocumentContent(documentId: string, tenantId: string, format: string) {
    const response = await this.audimodalClient.get(
      `/api/v1/tenants/${tenantId}/chunks`,
      {
        params: {
          file_id: documentId,
          order_by: "chunk_number",
        },
      }
    );

    const chunks = response.data.chunks || [];

    if (format === "chunks") {
      return chunks;
    }

    // Combine chunks into full text
    const fullText = chunks
      .sort((a: any, b: any) => a.chunk_number - b.chunk_number)
      .map((c: any) => c.content)
      .join("\n");

    return { content: fullText, chunk_count: chunks.length };
  }

  async searchDocuments(query: string, tenantId: string, notebookId?: string, topK: number = 10) {
    const response = await this.deeplakeClient.post(
      `/api/v1/datasets/documents/search/text`,
      {
        query_text: query,
        options: {
          top_k: topK,
          include_content: true,
          include_metadata: true,
        },
      },
      {
        headers: { "X-Tenant-ID": tenantId },
      }
    );

    return response.data.results || [];
  }

  async getDocumentSummary(documentId: string, tenantId: string) {
    // Try to get cached summary from metadata, otherwise return first chunk as preview
    const response = await this.audimodalClient.get(
      `/api/v1/tenants/${tenantId}/files/${documentId}`
    );

    const file = response.data;
    return {
      document_id: documentId,
      name: file.filename || "Unknown",
      size_bytes: file.size_bytes || 0,
      content_type: file.content_type || "unknown",
      chunk_count: file.chunk_count || 0,
      summary: file.summary || null,
      created_at: file.created_at,
    };
  }
}

// Create and configure the MCP server
const server = new Server(
  {
    name: "document-mcp",
    version: "1.0.0",
  },
  {
    capabilities: {
      tools: {},
      resources: {},
    },
  }
);

const tasClient = new TASClient();

// List available tools
server.setRequestHandler(ListToolsRequestSchema, async () => {
  return {
    tools: [
      {
        name: "list_notebook_documents",
        description: "List all documents in a notebook",
        inputSchema: {
          type: "object",
          properties: {
            notebook_id: {
              type: "string",
              description: "The ID of the notebook to list documents from",
            },
            include_metadata: {
              type: "boolean",
              description: "Include document metadata",
              default: true,
            },
          },
          required: ["notebook_id"],
        },
      },
      {
        name: "get_document_content",
        description: "Retrieve full content of a document",
        inputSchema: {
          type: "object",
          properties: {
            document_id: {
              type: "string",
              description: "The ID of the document to retrieve",
            },
            format: {
              type: "string",
              enum: ["text", "chunks"],
              description: "Output format - text for combined or chunks for individual",
              default: "text",
            },
          },
          required: ["document_id"],
        },
      },
      {
        name: "search_documents",
        description: "Semantic search across documents",
        inputSchema: {
          type: "object",
          properties: {
            query: {
              type: "string",
              description: "The search query",
            },
            notebook_id: {
              type: "string",
              description: "Optional notebook ID to filter search",
            },
            top_k: {
              type: "number",
              description: "Number of results to return",
              default: 10,
            },
          },
          required: ["query"],
        },
      },
      {
        name: "get_document_summary",
        description: "Get summary and metadata of a document",
        inputSchema: {
          type: "object",
          properties: {
            document_id: {
              type: "string",
              description: "The ID of the document to summarize",
            },
          },
          required: ["document_id"],
        },
      },
    ],
  };
});

// Handle tool calls
server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request.params;

  // Get tenant ID from environment or request context
  const tenantId = process.env.TENANT_ID || "default";

  try {
    switch (name) {
      case "list_notebook_documents": {
        const parsed = ListNotebookDocumentsSchema.parse(args);
        const documents = await tasClient.listNotebookDocuments(
          parsed.notebook_id,
          tenantId,
          parsed.include_metadata
        );
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(documents, null, 2),
            },
          ],
        };
      }

      case "get_document_content": {
        const parsed = GetDocumentContentSchema.parse(args);
        const content = await tasClient.getDocumentContent(
          parsed.document_id,
          tenantId,
          parsed.format
        );
        return {
          content: [
            {
              type: "text",
              text: typeof content === "string" ? content : JSON.stringify(content, null, 2),
            },
          ],
        };
      }

      case "search_documents": {
        const parsed = SearchDocumentsSchema.parse(args);
        const results = await tasClient.searchDocuments(
          parsed.query,
          tenantId,
          parsed.notebook_id,
          parsed.top_k
        );
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(results, null, 2),
            },
          ],
        };
      }

      case "get_document_summary": {
        const parsed = GetDocumentSummarySchema.parse(args);
        const summary = await tasClient.getDocumentSummary(parsed.document_id, tenantId);
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify(summary, null, 2),
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
        uri: "documents://recent",
        name: "Recent Documents",
        description: "List of recently accessed documents",
        mimeType: "application/json",
      },
    ],
  };
});

// Read resources
server.setRequestHandler(ReadResourceRequestSchema, async (request) => {
  const { uri } = request.params;

  if (uri === "documents://recent") {
    // Return a placeholder for recent documents
    return {
      contents: [
        {
          uri,
          mimeType: "application/json",
          text: JSON.stringify({ message: "Recent documents list not implemented yet" }),
        },
      ],
    };
  }

  throw new Error(`Unknown resource: ${uri}`);
});

// Main entry point
async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("Document MCP server running on stdio");
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exit(1);
});
