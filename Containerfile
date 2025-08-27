# syntax=docker/dockerfile:1
FROM golang:1.24 AS build
WORKDIR /app
COPY go.mod .
RUN go mod download
COPY . .
# Build static-ish binary for linux/amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/hermes ./cmd/hermes

FROM gcr.io/distroless/base-debian12
# OCI labels (baseline; CI may add/override via metadata-action)
ARG VERSION=""
ARG VCS_REF=""
ARG BUILD_DATE=""
LABEL org.opencontainers.image.title="Hermes" \
      org.opencontainers.image.description="A lightweight, multi-tenant object storage console and API." \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"

WORKDIR /
COPY --from=build /out/hermes /hermes
COPY openapi /openapi
COPY web /web
ENV PORT=8080
EXPOSE 8080
# Run as non-root by default (OpenShift/OKD may override with arbitrary UID)
USER nonroot:nonroot
ENTRYPOINT ["/hermes"]
