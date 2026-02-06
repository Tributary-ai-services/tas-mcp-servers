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
ENV DEEPLAKE_URL=http://tas-deeplake-api:8000
ENV AUDIMODAL_URL=http://tas-audimodal:8084
ENV AETHER_URL=http://tas-aether-backend:8080

# Run the server
CMD ["node", "dist/index.js"]
