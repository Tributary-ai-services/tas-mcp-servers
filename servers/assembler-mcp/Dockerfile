FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /assembler-mcp .

FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /assembler-mcp /app/assembler-mcp
COPY templates/ /app/templates/

EXPOSE 8091

ENV PORT=8091
ENV TEMPLATES_DIR=/app/templates

ENTRYPOINT ["/app/assembler-mcp"]
