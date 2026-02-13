# TAS MCP HTTP Bridge
# Generic stdio-to-HTTP bridge for MCP servers.
#
# Build:
#   docker build -t registry-api.tas.scharber.com/tas-mcp-bridge:1.0.0 .
#
# Run:
#   docker run -e MCP_COMMAND=npx -e MCP_ARGS="-y @upstash/context7-mcp" \
#     -p 8000:8000 registry-api.tas.scharber.com/tas-mcp-bridge:1.0.0

FROM node:20-slim

WORKDIR /app

# Install common tools that stdio MCP servers may need
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Copy bridge source and build
COPY package.json tsconfig.json ./
RUN npm install

COPY src/ ./src/
RUN npm run build

# Default port
ENV BRIDGE_PORT=8000

EXPOSE 8000

# Pre-install common MCP server packages for faster startup
# These are cached in the image so npx doesn't need to download them
RUN npx -y @upstash/context7-mcp --help 2>/dev/null || true
RUN npx -y @modelcontextprotocol/server-sequential-thinking --help 2>/dev/null || true

CMD ["node", "dist/index.js"]
