# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder
WORKDIR /opt/app-root/src
# Build-time args to control Go module fetching behavior (default: bypass proxy)
ARG GOPROXY=direct
ARG GOSUMDB=off
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}
# Cache go modules
COPY go.mod .
COPY go.sum .
RUN go mod download
# Copy source
COPY . .
RUN mkdir -p out
# Build static binary (CGO disabled for minimal runtime deps)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -o ./out/hermes ./cmd/hermes

# ---- Runtime stage ----
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.4
WORKDIR /app
# Install curl for healthcheck and ensure CA certs
#RUN microdnf -y update && microdnf -y install curl ca-certificates && microdnf -y clean all
# Create non-root user
RUN useradd -u 10001 -r -m -s /sbin/nologin appuser || true
# Copy binary and templates
COPY --from=builder /opt/app-root/src/out/hermes /app/hermes
COPY templates /app/templates
ENV PORT=8080
EXPOSE 8080
USER appuser
#HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1
CMD ["/app/hermes"]
