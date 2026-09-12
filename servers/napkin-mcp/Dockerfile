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

WORKDIR /app

# Copy built files
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
COPY package.json ./

# Environment variables
ENV NODE_ENV=production
ENV NAPKIN_API_BASE_URL=https://api.napkin.ai
ENV NAPKIN_POLLING_INTERVAL=3000
ENV NAPKIN_MAX_WAIT_TIME=300000
ENV MINIO_ENDPOINT=http://tas-minio-shared:9000
ENV MINIO_BUCKET=napkin-visuals
ENV HEALTH_PORT=8087

EXPOSE 8087

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget --spider -q http://localhost:8087/health || exit 1

CMD ["node", "dist/index.js"]
