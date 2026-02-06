# Document MCP Server

A Model Context Protocol (MCP) server that provides document retrieval and context injection capabilities for TAS agents.

## Features

- **list_notebook_documents**: List all documents in a notebook
- **get_document_content**: Retrieve full content of a document
- **search_documents**: Semantic search across documents using DeepLake vector search
- **get_document_summary**: Get cached summary of a document

## Installation

```bash
npm install
npm run build
```

## Configuration

Set the following environment variables:

```bash
# TAS Service URLs
DEEPLAKE_URL=http://localhost:8000
AUDIMODAL_URL=http://localhost:8084
AETHER_URL=http://localhost:8080

# Authentication
TAS_API_KEY=your-api-key

# Tenant context
TENANT_ID=your-tenant-id
```

## Usage

### As a standalone server

```bash
npm run start
```

### With Claude Desktop

Add to your Claude Desktop configuration (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "document-mcp": {
      "command": "node",
      "args": ["/path/to/tas-mcp-servers/servers/document-mcp/dist/index.js"],
      "env": {
        "DEEPLAKE_URL": "http://localhost:8000",
        "AUDIMODAL_URL": "http://localhost:8084",
        "AETHER_URL": "http://localhost:8080",
        "TAS_API_KEY": "your-api-key",
        "TENANT_ID": "your-tenant-id"
      }
    }
  }
}
```

### With TAS MCP Federation

Register this server with the TAS MCP federation for centralized access and management.

## Tools

### list_notebook_documents

List all documents in a notebook.

**Parameters:**
- `notebook_id` (string, required): The ID of the notebook
- `include_metadata` (boolean, optional): Include document metadata (default: true)

### get_document_content

Retrieve the full content of a document.

**Parameters:**
- `document_id` (string, required): The ID of the document
- `format` (string, optional): Output format - "text" for combined or "chunks" for individual (default: "text")

### search_documents

Perform semantic search across documents.

**Parameters:**
- `query` (string, required): The search query
- `notebook_id` (string, optional): Filter by notebook ID
- `top_k` (number, optional): Number of results to return (default: 10)

### get_document_summary

Get summary and metadata of a document.

**Parameters:**
- `document_id` (string, required): The ID of the document

## Integration with TAS Agent Builder

This MCP server enables agents to autonomously retrieve document context by invoking these tools through the TAS MCP federation. Agents can decide when and what documents to retrieve based on user queries.

Example agent prompt:
```
You have access to document retrieval tools. Use the search_documents tool to find relevant information before answering questions about the user's documents.
```

## Development

```bash
# Run in development mode
npm run dev

# Lint code
npm run lint

# Run tests
npm run test
```
