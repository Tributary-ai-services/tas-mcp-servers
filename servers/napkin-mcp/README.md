# Napkin AI MCP Server

MCP server that integrates Napkin AI visual generation into the TAS platform. Generates visuals from text content, stores results in MinIO, and exposes capabilities via the Model Context Protocol for federation.

## Tools

| Tool | Description |
|------|-------------|
| `generate_visual` | Submit text, wait for processing, download result, store in MinIO |
| `check_visual_status` | Check status of a pending generation request |
| `download_visual` | Download a generated visual from MinIO |
| `list_styles` | List available Napkin AI visual styles |
| `list_visuals` | List generated visuals stored in MinIO |
| `create_napkin_visual_cr` | Generate a NapkinVisual K8s CR manifest |

## Resources

| URI | Description |
|-----|-------------|
| `napkin://styles` | Available visual styles |
| `napkin://visuals/recent` | Recently generated visuals |

## Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `NAPKIN_API_KEY` | | Yes | Bearer token for Napkin AI API |
| `NAPKIN_API_BASE_URL` | `https://api.napkin.ai` | No | Napkin API base URL |
| `NAPKIN_POLLING_INTERVAL` | `3000` | No | Polling interval in ms |
| `NAPKIN_MAX_WAIT_TIME` | `300000` | No | Max wait time in ms |
| `MINIO_ENDPOINT` | `http://tas-minio-shared:9000` | No | MinIO endpoint |
| `MINIO_ACCESS_KEY` | `minioadmin` | No | MinIO access key |
| `MINIO_SECRET_KEY` | `minioadmin123` | No | MinIO secret key |
| `MINIO_BUCKET` | `napkin-visuals` | No | Default MinIO bucket |
| `HEALTH_PORT` | `8087` | No | Health check HTTP port |

## Development

```bash
# Install dependencies
npm install

# Build
npm run build

# Run locally
NAPKIN_API_KEY=your-key npm start

# Run with Docker Compose
NAPKIN_API_KEY=your-key docker-compose up -d
```

## Health Check

```bash
curl http://localhost:8087/health
```

## Port

- **8087**: Health check HTTP endpoint
