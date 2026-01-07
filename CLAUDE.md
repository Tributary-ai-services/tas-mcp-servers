# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**TAS MCP Servers** is a collection of pre-built Model Context Protocol (MCP) servers ready for local deployment and integration with the TAS MCP federation platform. These servers provide standardized interfaces for various capabilities including search, web scraping, database access, and development tools.

## Data Models & Schema Reference

### Service-Specific Data Models
This service's data models and server configurations are documented in the centralized data models repository:

**Location**: `../aether-shared/data-models/tas-mcp-servers/`

#### MCP Server Configurations:
The data models for TAS MCP Servers are organized in the following subdirectories:
- **`servers/`** - Individual MCP server configurations and schemas
- **`integrations/`** - Integration patterns and connection specifications

These configurations define the protocol interfaces and capabilities for each supported MCP server.

#### Cross-Service Integration:
- **TAS MCP Federation** (`../aether-shared/data-models/tas-mcp/`) - Protocol definitions and server registry
- **Platform ERD** (`../aether-shared/data-models/cross-service/diagrams/platform-erd.md`) - Complete entity relationship diagram
- **Architecture Overview** (`../aether-shared/data-models/cross-service/diagrams/architecture-overview.md`) - MCP servers in system architecture

#### When to Reference Data Models:
1. Before adding new MCP servers to the collection
2. When implementing server-specific configurations or capabilities
3. When debugging MCP server integration or protocol issues
4. When onboarding new developers to understand MCP server architecture
5. Before modifying server schemas or protocol implementations

**Main Documentation Hub**: `../aether-shared/data-models/README.md` - Complete navigation for all 38 data model files

## Supported MCP Servers

This repository includes pre-configured MCP servers for local deployment:

### Search Servers
- **DuckDuckGo Search**: Privacy-focused web search with zero tracking
- **Brave Search**: Alternative privacy-focused search engine

### Web Scraping Servers
- **Apify Integration**: Access to 5,000+ scraping actors
- **Custom Scrapers**: Domain-specific web scraping tools

### Database Servers
- **PostgreSQL MCP**: Secure database access with query controls
- **Redis MCP**: Key-value store access for caching and sessions

### Development Tool Servers
- **Git MCP**: Repository automation and management
- **File System MCP**: Local file system access with safety controls

## Integration with TAS MCP

These servers are designed to work seamlessly with the TAS MCP federation platform:

1. **Server Registration**: Servers are automatically registered with the MCP federation
2. **Protocol Compliance**: All servers implement the standard Model Context Protocol
3. **Health Monitoring**: Built-in health checks for federation management
4. **Configuration Management**: Centralized configuration via TAS MCP

## Configuration

Each MCP server can be configured independently:

```yaml
# Example server configuration
server:
  name: search-server
  type: brave-search
  enabled: true
  capabilities:
    - search
    - suggest
  config:
    api_key: ${BRAVE_API_KEY}
    max_results: 10
```

## Deployment

### Local Development
```bash
# Start individual server
npm install
npm run start:server-name

# Or using Docker
docker-compose up server-name
```

### TAS MCP Integration
```bash
# Servers are automatically discovered by TAS MCP when running in the shared network
docker-compose --profile mcp-servers up -d
```

## Integration Points

- **TAS MCP**: Primary federation platform for unified access
- **TAS Agent Builder**: MCP servers provide tools for agent capabilities
- **TAS Workflow Builder**: MCP servers enable workflow step execution
- **Aether Backend**: MCP capabilities for document processing and search

## Important Notes

- All MCP servers implement the standardized Model Context Protocol
- Servers are designed for local deployment and federation via TAS MCP
- Configuration is managed centrally through TAS MCP when federated
- Health checks ensure automatic failover in the federation
- Integration with TAS infrastructure via `tas-shared-network` Docker network
- Server registry in TAS MCP includes 1,535+ community MCP servers for extensibility

## Adding New MCP Servers

To add a new MCP server to the collection:

1. Create server configuration in `servers/` directory
2. Document server schema in `../aether-shared/data-models/tas-mcp-servers/servers/`
3. Add integration tests for protocol compliance
4. Update Docker Compose configuration for deployment
5. Register server with TAS MCP federation

## Available Documentation

- **TAS MCP README**: `../tas-mcp/README.md` - Federation platform documentation
- **MCP Specification**: Model Context Protocol standard and implementation guide
- **Data Models**: `../aether-shared/data-models/tas-mcp/` - Protocol definitions
