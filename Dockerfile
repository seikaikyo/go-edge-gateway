# Multi-stage build for edge-gateway
#
# go-factory-io module path is github.com/dashfactory/go-factory-io
# but the repo lives at github.com/seikaikyo/go-factory-io.
# We use git insteadOf to redirect the module fetch, and strip
# the local replace directive so Go fetches from GitHub.

FROM golang:1.26-alpine AS builder

RUN apk --no-cache add git
RUN git config --global url."https://github.com/seikaikyo/go-factory-io".insteadOf "https://github.com/dashfactory/go-factory-io"

ENV GONOSUMCHECK=github.com/dashfactory/go-factory-io
ENV GOFLAGS=-mod=mod

WORKDIR /src
COPY go.mod go.sum ./

# Remove local replace directive for Docker build
RUN sed -i '/^replace.*go-factory-io/d' go.mod
RUN go mod tidy && go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /edge-gateway ./cmd/edge-gateway/

# --- Production image ---
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /edge-gateway /usr/local/bin/edge-gateway
COPY edge-gateway.demo.yaml /etc/edge-gateway/config.yaml

EXPOSE 8080

ENTRYPOINT ["edge-gateway"]
CMD ["--config", "/etc/edge-gateway/config.yaml"]
