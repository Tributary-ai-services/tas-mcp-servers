FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -o /datasource-operator ./cmd/operator/

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /datasource-operator /datasource-operator
USER 65532:65532

ENTRYPOINT ["/datasource-operator"]
