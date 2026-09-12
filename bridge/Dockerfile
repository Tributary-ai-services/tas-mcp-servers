FROM node:20-slim

# Install Python and uv for Python-based MCP servers, plus npx/uvx support
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip curl && \
    pip3 install --break-system-packages uv && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY package.json ./
RUN npm install --production 2>/dev/null || true

COPY src/ ./src/

ENV BRIDGE_PORT=8000
EXPOSE 8000

CMD ["node", "src/server.js"]
