FROM node:20-alpine AS builder

WORKDIR /app

# Copy package files
COPY package.json ./
RUN npm install

# Copy source and build
COPY tsconfig.json ./
COPY src ./src
RUN npm run build

# Production image
FROM node:20-alpine

# Install FFmpeg
RUN apk add --no-cache ffmpeg

WORKDIR /app

# Copy built files
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
COPY package.json ./

# Create temp directory for audio processing
RUN mkdir -p /tmp/podcast-mcp

# Environment variables
ENV NODE_ENV=production
ENV MINIO_ENDPOINT=http://tas-minio-shared:9000
ENV MINIO_ASSETS_BUCKET=podcast-assets
ENV MINIO_OUTPUT_BUCKET=podcast-output
ENV KOKORO_API_URL=http://kokoro-tts.tas-mcp-servers.svc.cluster.local:8880
ENV HEALTH_PORT=8092
ENV TTS_CONCURRENCY=2
ENV SEGMENT_SIZE=12

EXPOSE 8092

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget --spider -q http://localhost:8092/health || exit 1

CMD ["node", "--expose-gc", "dist/index.js"]
