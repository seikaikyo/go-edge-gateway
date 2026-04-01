# Multi-stage build for edge-gateway
#
# NOTE: go.mod uses "replace" to reference ../go-factory-io locally.
# For Docker build, copy both repos into the build context:
#
#   docker build -f go-edge-gateway/Dockerfile \
#     --build-context factory=go-factory-io \
#     -t edge-gateway go-edge-gateway
#
# Or publish go-factory-io as a Go module and remove the replace directive.

FROM golang:1.26-alpine AS builder

WORKDIR /src

# Copy go-factory-io dependency first (from build context or parent)
COPY --from=factory . /go-factory-io

WORKDIR /src/edge-gateway
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /edge-gateway ./cmd/edge-gateway/

# --- Production image ---
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /edge-gateway /usr/local/bin/edge-gateway

EXPOSE 8080

ENTRYPOINT ["edge-gateway"]
CMD ["--config", "/etc/edge-gateway/config.yaml"]
