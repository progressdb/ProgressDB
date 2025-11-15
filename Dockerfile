## Multi-stage Dockerfile for ProgressDB
# Builds a static Go binary and packages it into a minimal runtime image.

### Build stage
FROM golang:1.24 AS builder
WORKDIR /src

# Copy only essential files from service for efficient build context
COPY service/go.mod service/go.sum /src/service/
COPY service/cmd /src/service/cmd
COPY service/internal /src/service/internal
COPY service/pkg /src/service/pkg

# Copy KMS module for local dependency
COPY kms /src/kms
WORKDIR /src/service
ARG VERSION=dev
ARG COMMIT=none
ARG BUILDDATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILDDATE}" \
    -o /out/progressdb ./cmd/progressdb

### Runtime stage
FROM debian:bookworm-slim

RUN groupadd --gid 1000 progressdb && \
    useradd --uid 1000 --gid 1000 --create-home --home-dir /home/progressdb progressdb

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/progressdb /usr/local/bin/progressdb
RUN chmod +x /usr/local/bin/progressdb


# Copy config file to temp location, will be moved to /data during startup
COPY scripts/config.yaml /tmp/config.yaml

USER progressdb
WORKDIR /home/progressdb

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s \
  CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/progressdb"]
CMD ["--config", "/tmp/config.yaml"]
