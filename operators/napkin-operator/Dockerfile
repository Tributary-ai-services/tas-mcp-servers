FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /napkin-operator ./cmd/operator/

# Production image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /napkin-operator /napkin-operator

USER 65532:65532

ENTRYPOINT ["/napkin-operator"]
