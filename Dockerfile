## Multi-stage Dockerfile for Hermes
# Build stage
FROM golang:1.24-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src

# Cache modules first
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source
COPY . .

# Build static binary for target OS/ARCH (set by Buildx)
ARG TARGETOS
ARG TARGETARCH
# Build metadata (can be passed by buildx)
ARG VERSION=dev
ARG VCS_REF=""
ARG BUILD_DATE=""
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "-s -w -X github.com/arencloud/hermes/internal/version.Version=${VERSION} -X github.com/arencloud/hermes/internal/version.Commit=${VCS_REF} -X github.com/arencloud/hermes/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/server ./cmd/server

# Copy static assets
RUN mkdir -p /out/web/dist && cp -r web/dist/* /out/web/dist/

# Runtime stage
FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app \
    && apk add --no-cache ca-certificates tzdata \
    && mkdir -p /data
WORKDIR /app

COPY --from=build /out/server ./server
COPY --from=build /out/web ./web

# Ensure non-root can write to /data if using sqlite
RUN chown -R app:app /data /app
USER app

ENV HTTP_PORT=8080 \
    STATIC_DIR=/app/web/dist \
    DB_DRIVER=sqlite \
    DB_PATH=/data/hermes.db

EXPOSE 8080
ENTRYPOINT ["./server"]
