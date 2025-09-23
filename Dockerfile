# syntax=docker/dockerfile:1.7

# -------- Builder stage --------
FROM golang:1.24-alpine AS builder
# Needed for CGO (sqlite) and faster crypto
RUN apk add --no-cache git build-base
WORKDIR /src

# Enable Go modules caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Build-time metadata (passed from workflow)
ARG VERSION
ARG VCS_REF
ARG BUILD_DATE

# Build the server binary. CGO is required for sqlite (go-sqlite3)
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/server ./cmd/server

# -------- Runtime stage --------
FROM alpine:3.20

# Non-root user
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app

# Copy binary and static assets
COPY --from=builder /out/server /app/server
COPY web/dist /app/web/dist
COPY img/logo /app/img/logo

# Build-time metadata again for labels
ARG VERSION
ARG VCS_REF
ARG BUILD_DATE

# OCI labels
LABEL org.opencontainers.image.title="Hermes" \
      org.opencontainers.image.description="Hermes server" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"

# Sensible defaults for running in container
ENV APP_ENV=prod \
    HTTP_PORT=8080 \
    STATIC_DIR=/app/web/dist \
    DB_DRIVER=sqlite \
    DB_PATH=/data/hermes.db

# Writable data directory (mount a volume / PVC in prod)
RUN mkdir -p /data && chown -R app:app /data /app
VOLUME ["/data"]

EXPOSE 8080
USER app

ENTRYPOINT ["/app/server"]
