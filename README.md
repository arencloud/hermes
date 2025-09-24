<div align="center">

<img src="img/logo/logo.png" alt="Hermes Logo" width="120" />

<h1>Hermes</h1>

<i>A lightweight S3‑compatible storage manager and API with built‑in observability.</i>

<p>
  <a href="LICENSE"><img alt="License: Apache-2.0" src="https://img.shields.io/badge/License-Apache_2.0-blue.svg"></a>
  <a href=".github/workflows/release.yml"><img alt="CI: Release" src="https://img.shields.io/github/actions/workflow/status/arencloud/hermes/release.yml?label=release"></a>
  <a href="https://ghcr.io/arencloud/hermes"><img alt="Container" src="https://img.shields.io/badge/Container-GHCR-blue"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white">
  <a href="deploy/helm/hermes/README.md"><img alt="Helm Chart" src="https://img.shields.io/badge/Helm-Chart-0F1689?logo=helm&logoColor=white"></a>
</p>

<p>
  <a href="#overview">Overview</a> •
  <a href="#quickstart">Quickstart</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#api-overview">API</a> •
  <a href="#observability">Observability</a> •
  <a href="#security">Security</a> •
  <a href="#helm-chart">Helm</a> •
  <a href="#license">License</a>
</p>

</div>

---

## Overview ✨

Hermes is a small Go service that helps you manage S3‑compatible storage providers (e.g., MinIO, AWS S3), buckets and objects through a simple web UI plus a clean HTTP API. It includes structured logging, request tracing, and basic operational metrics out of the box.

Highlights:
- S3‑compatible providers: MinIO, AWS S3, and other S3 endpoints
- Manage Providers, Buckets, and Objects via REST API and Web UI
- File uploads/downloads, list/delete, copy/move across buckets/providers
- Built‑in auth with roles (viewer/editor/admin). OIDC ready
- Observability: request tracing, structured logs, lightweight metrics
- Works out of the box with SQLite; switch to Postgres without code changes
- Containerized, non‑root runtime, read‑only root filesystem by default
- Helm chart for Kubernetes/OKD/OpenShift

## Architecture 🧩

- Go HTTP server using chi router and CORS middleware
- Storage abstraction via minio-go SDK (S3 protocol)
- Persistence via GORM with SQLite (default) or Postgres
- No embedded assets: static UI is served from disk (STATIC_DIR)
- Observability: request traces stored in DB; logs persisted optionally

Key directories:
- cmd/server: main entrypoint
- internal/api: routing, endpoints, auth, tracing
- internal/db: DB init, migrations, logging persistence, bootstrap
- internal/models: GORM models
- web/dist: static web UI assets (served as files)
- deploy/helm/hermes: Helm chart

## Quickstart 🚀

Choose one of the following ways to run Hermes.

### 1) Run locally with Podman Compose (MinIO + Postgres)

Requirements: podman, podman-compose, curl, jq, Go (optional)

- Start dependencies:
  make dev-up

- Build and run Hermes locally:
  make run

- Wait until Hermes is healthy:
  make wait-server

- Provision a local MinIO provider via API:
  make provision-minio

- Create a demo bucket and upload a sample file:
  make demo-bucket
  make demo-upload

Hermes API base will be http://127.0.0.1:8080 (override HTTP_PORT).
MinIO defaults: http://127.0.0.1:9000 (console at 9001).

Stop stack:
  make dev-down

Useful:
  make dev-logs S=minio
  make dev-logs S=postgres

### 2) Docker/Podman (container only)

Pull the image from GHCR:
  docker pull ghcr.io/arencloud/hermes:latest

Run with a writable data volume for SQLite (default):
  docker run --rm -p 8080:8080 \
    -e DB_DRIVER=sqlite -e DB_PATH=/data/hermes.db \
    -v hermes-data:/data \
    ghcr.io/arencloud/hermes:latest

Environment defaults inside the image:
- APP_ENV=prod
- HTTP_PORT=8080
- STATIC_DIR=/app/web/dist
- DB_DRIVER=sqlite
- DB_PATH=/data/hermes.db

### 3) Kubernetes/OKD/OpenShift via Helm

The chart is included at deploy/helm/hermes.

- Add image tag (example vX.Y.Z) and install:
  helm upgrade --install hermes deploy/helm/hermes \
    --set image.repository=ghcr.io/arencloud/hermes \
    --set image.tag=vX.Y.Z

- Expose it with Ingress or OpenShift Route (see values.yaml for toggles):
  # Ingress example:
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=hermes.local \
  --set ingress.hosts[0].paths[0].path=/

- Persistence: by default enabled for SQLite at /data; configure PVC in values.yaml.
- Security: runs as non‑root; container root filesystem is read‑only by default.

## Configuration ⚙️

Hermes is configured via environment variables (see internal/config/config.go):
- APP_ENV: dev|prod (default: dev)
- HTTP_PORT: listening port (default: 8080)
- DB_DRIVER: sqlite|postgres (default: sqlite)
- DB_PATH: path to SQLite DB when DB_DRIVER=sqlite (default: data/hermes.db)
- DATABASE_URL / DB_DSN: Postgres DSN when DB_DRIVER=postgres (e.g. postgres://user:pass@host:5432/db?sslmode=disable)
- STATIC_DIR: static assets directory (default: web/dist; in container: /app/web/dist)
- MAX_UPLOAD_SIZE_BYTES: per-request upload cap; 0 = unlimited (default: 0). Enforced for multipart uploads to prevent OOM.

Selected runtime toggles from Helm values (deploy/helm/hermes/values.yaml):
- service.port: Service port (default 8080)
- http.containerPort: Container port (maps to HTTP_PORT)
- persistence.*: PVC settings and mount path (/data)
- db.driver, db.sqlitePath, db.dsn: DB options
- extraEnv: extra environment variables (e.g., LOG_LEVEL, LOG_JSON, OIDC_* settings)
- containerSecurityContext.readOnlyRootFilesystem: true by default

## Database 💾

SQLite (default):
- Zero-config. The DB file is created automatically.
- For containers, mount a volume at /data and use DB_PATH=/data/hermes.db

Postgres:
- Set DB_DRIVER=postgres and DATABASE_URL to your DSN.
- Helm: configure db.driver=postgres and db.dsn accordingly; disable persistence.

On first run, Hermes auto-migrates the schema and bootstraps:
- One auth config row (mode: local, disabled by default)
- A default admin user if no users exist:
  - email: admin@local
  - temp password: generated and logged once (look for "default admin created" in logs)

## API Overview 🔗

Base paths:
- Health: GET /health → "ok"
- Version: GET /api/version → { name: "hermes", version: "<version>" }
- Main API: /api/v1 (requires authentication for most endpoints)

Auth & Users:
- POST /api/v1/auth/login { email, password }
- GET  /api/v1/auth/me
- Admin-only user management:
  - GET  /api/v1/users/
  - POST /api/v1/users/ { email, password, role }
  - PUT  /api/v1/users/{id}
  - DELETE /api/v1/users/{id}

Providers & Buckets:
- GET  /api/v1/providers
- POST /api/v1/providers { name, type, endpoint, accessKey, secretKey, region, useSSL }
- GET  /api/v1/providers/{id}
- PUT  /api/v1/providers/{id}
- DELETE /api/v1/providers/{id}
- GET  /api/v1/providers/{id}/buckets
- POST /api/v1/providers/{id}/buckets { name, region }

Objects:
- GET    /api/v1/providers/{id}/buckets/{name}/objects?prefix=&recursive=
- POST   /api/v1/providers/{id}/buckets/{name}/upload (multipart form: file, key)
- GET    /api/v1/providers/{id}/buckets/{name}/download?key=
- DELETE /api/v1/providers/{id}/buckets/{name}/objects?key=
- POST   /api/v1/providers/{id}/buckets/{name}/copy { srcKey, dstBucket, dstKey?, dstProviderId? } (NDJSON progress)
- POST   /api/v1/providers/{id}/buckets/{name}/move { srcKey, dstBucket, dstKey?, dstProviderId? } (NDJSON progress)

Observability & Logs:
- GET /api/v1/obs/metrics → lightweight metrics snapshot
- GET /api/v1/obs/summary → summarized request stats
- GET /api/v1/obs/errors → recent 4xx/5xx traces
- GET /api/v1/trace/recent, GET /api/v1/trace/{id}
- GET /api/v1/logs/recent, GET /api/v1/logs/download
- GET /api/v1/logs/level, PUT /api/v1/logs/level
- Web UI and assets available under /

OpenAPI:
- GET /api/v1/openapi.json (requires editor/admin). Minimal OpenAPI 3.0 spec

## Authentication & Roles 🔐

- Local auth (email/password). Default first admin is created on empty DB.
- Roles: viewer, editor, admin. Certain endpoints are restricted (e.g., users/* requires admin; openapi.json requires editor/admin).
- OIDC support is planned/available in codebase; configure via extraEnv values (e.g., issuer, client ID/secret) when enabling.

## Observability 📈

Hermes attaches a per-request trace with an X-Trace-Id header.
- Traces are stored in the database (Trace and TraceEvent rows)
- Structured logs include traceId, method, path, status, timing, and sizes
- Simple counters exposed via /api/v1/obs/metrics and /api/v1/obs/summary

## Security 🛡️

- Container runs as non‑root, with a read‑only root filesystem by default (Helm values)
- CORS is permissive by default for demo convenience; lock down in front proxy for production
- Health and static UI are public; operational endpoints require auth

## Build from source 🛠️

Prerequisites: Go 1.24+

- Build:
  make build

- Run (SQLite, local assets):
  make run

- Test:
  make test

Server binary is built at ./server (git-ignored).

## Docker image 📦

Multi-arch image is published to GHCR by the Release workflow when tags vX.Y.Z are pushed:
- ghcr.io/arencloud/hermes:<version>
- ghcr.io/arencloud/hermes:v<major>.<minor>
- ghcr.io/arencloud/hermes:v<major>
- ghcr.io/arencloud/hermes:latest

Runtime defaults in the image:
- STATIC_DIR=/app/web/dist
- DB_PATH=/data/hermes.db (mount /data)
- Static assets and the logo are baked into the image at /app/web/dist and /app/img/logo

Versioning and Web UI:
- The server exposes GET /api/version returning the application version.
- The Web UI reads /api/version and displays it in the Observability section, ensuring the UI shows the same version as the running image.
- The version is injected at build time via Go ldflags and Docker ARG VERSION. Release builds pass the tag (e.g., v0.1.3), so /api/version matches the image tag.

## Helm chart ⛵

See deploy/helm/hermes for full chart. Key values in values.yaml:
- image.repository, image.tag
- http.containerPort, service.port
- persistence.* for SQLite PVC
- db.driver/sqlitePath/dsn
- ingress and openshift.route toggles
- security contexts for pod and container

Notes are printed after install/upgrade via templates/NOTES.txt.

## Troubleshooting 🧰

- I can’t log in: Check logs for the default admin credentials on first run; ensure DB is writable when using SQLite.
- Static UI not loading: Confirm STATIC_DIR path and that web/dist exists. In container, it’s baked into /app/web/dist.
- Postgres connection errors: Verify DATABASE_URL (or DB_DSN) and network reachability; set db.driver=postgres in Helm.
- MinIO provider errors: Ensure MinIO is healthy (make wait-minio), correct endpoint/credentials, and useSSL flag.
- Uploading large files on Kubernetes/OKD/OpenShift causes 413/crash: set NGINX ingress annotations `nginx.ingress.kubernetes.io/proxy-body-size: "0"` and `nginx.ingress.kubernetes.io/proxy-request-buffering: "off"`; for OpenShift Route, there’s no per-route body size limit by default, but ensure any third-party ingress in front allows large bodies. Optionally cap in-app via `MAX_UPLOAD_SIZE_BYTES` or Helm `upload.maxBodyBytes`.

## Contributing 🤝

Issues and PRs are welcome. Please run make test and include context for changes. Keep the container non‑root and secure‑by‑default.

## License 📄

Hermes is released under the Apache License 2.0. See [LICENSE](LICENSE) for details.
