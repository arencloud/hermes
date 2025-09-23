<div align="center">

  <img alt="Hermes" src="img/logo/logo.png" width="160"/>

  <h1>Hermes</h1>
  <p><i>Lightweight, batteries‑included S3 explorer and gateway</i></p>

  <p>
    <img alt="Go" src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go" />
    <img alt="Platforms" src="https://img.shields.io/badge/platforms-amd64%20%7C%20arm64-6C757D" />
    <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-Apache--2.0-blue" /></a>
  </p>

  <p>
    Connect to S3‑compatible providers (AWS, MinIO, MCG, generic S3), browse buckets/objects, upload/download files, and manage access. 
    Ships with local/OIDC auth, structured logs, request tracing, metrics, and a clean API behind a single‑page app.
  </p>

</div>

---

- Website/UI: Single‑page app served directly by Hermes
- API: JSON over HTTP at /api/v1
- Status: Early preview, evolving rapidly

---

## Table of contents

- Overview
- Highlights
- Quick start
  - Docker
  - Helm (Kubernetes/OpenShift)
  - Podman compose (local dev)
- Configuration
- Database
- Features in depth
- API overview
- Development
- Containers and releases
- Security
- Roadmap
- Contributing
- License

---

## Highlights

- 🌐 S3‑compatible providers: AWS S3, MinIO, MCG, and generic endpoints
- 🧰 Multi‑provider, multi‑bucket management
- ⬆️⬇️ Upload/download objects via API (streaming)
- 🔐 Authentication: Local users + OIDC (OpenID Connect)
- 👥 Roles: admin, editor, viewer
- 🛡️ Secure by default: Non‑root container, scoped CORS, structured logs
- 📈 Observability: Request tracing, recent logs, metrics, health checks
- 💾 Persistence: SQLite (default) or PostgreSQL
- 🧱 Self‑contained image, multi‑architecture (linux/amd64, linux/arm64)
- 🚀 Built with Go 1.24

---

## Quick start

### Docker

- Pull image (published on GHCR):
  
  docker pull ghcr.io/arencloud/hermes:latest

- Run (SQLite by default):
  
  docker run --rm -p 8080:8080 \
    -e DB_DRIVER=sqlite \
    -e DB_PATH=/data/hermes.db \
    -v $(pwd)/data:/data \
    ghcr.io/arencloud/hermes:latest

- Open http://localhost:8080

Note: On first boot, Hermes auto‑creates an admin user with a random temporary password and logs it at INFO level. Change it immediately.

### Helm (Kubernetes/OpenShift)

A Helm chart is included under deploy/helm/hermes.

- Values reference: deploy/helm/hermes/values.yaml
- Example install (namespace hermes):
  
  helm upgrade --install hermes ./deploy/helm/hermes -n hermes --create-namespace \
    --set image.repository=ghcr.io/arencloud/hermes \
    --set image.tag=latest

See chart README for ingress/route and persistence options.

### Podman compose (local dev stack)

Start Postgres and MinIO locally (optional) using podman-compose and then run Hermes from your host:

- Start deps:
  
  make dev-up

- Run Hermes locally:
  
  make run

- Provision a demo MinIO provider and bucket:
  
  make provision-minio
  
  make demo-bucket
  
  make demo-upload

---

## Configuration

Hermes reads configuration from environment variables (see internal/config/config.go):

- APP_ENV: dev|prod (default: dev)
- HTTP_PORT: HTTP server port (default: 8080)
- DB_DRIVER: sqlite|postgres (default: sqlite)
- DB_PATH: path to SQLite DB file (default: data/hermes.db)
- DATABASE_URL or DB_DSN: Postgres connection string when DB_DRIVER=postgres
- STATIC_DIR: directory for static web assets (default: web/dist)
- LOG_LEVEL: debug|info|error|fatal (default: info)
- LOG_JSON: true|false (default: true)

### Auth configuration (OIDC via API)

Auth configuration is stored in the database and can be managed via the /api/v1/auth/config endpoints. Typical OIDC fields include issuer, client ID/secret, scope, redirect URL, and claim mappings for roles/groups.

---

## Database

- Default: SQLite at DB_PATH (persist to a mounted volume in containers, e.g., /data)
- PostgreSQL: Set DB_DRIVER=postgres and provide DATABASE_URL
- Auto‑migrations: GORM auto‑migrates the schema on startup
- Bootstrap: If no users exist, Hermes creates a default admin user with a temporary password and logs it

---

## Features in depth

### Providers and buckets

- Register multiple S3 providers with endpoint/credentials
- Create and list buckets per provider
- Upload objects via multipart/form-data or direct streams
- Download objects with range support
- Optional region and SSL flags per provider

### Authentication and authorization

- Local users with bcrypt‑hashed passwords
- OIDC login flow with claim mapping for roles (admin/editor/viewer)
- Session management via secure cookies
- Force password change for bootstrapped admin on first login

### Observability

Hermes instruments requests end‑to‑end and persists:
- Structured logs with fields (level, msg, fields)
- Traces per request, with events and durations
- Metrics (process and HTTP): /api/v1/metrics
- Recent errors: /api/v1/errors
- Recent logs: /api/v1/logs/recent
- Health check: /health

### Static assets and SPA routing

- SPA build is served from STATIC_DIR
- Unknown routes fall back to index.html
- Raw image assets served under /img

---

## API overview

Base URL: http://localhost:8080

- Health and version:
  
  GET /health → "ok"
  
  GET /api/version → { name, version }

- Auth (representative endpoints):
  
  POST /api/v1/auth/login
  
  POST /api/v1/auth/logout
  
  GET  /api/v1/auth/me
  
  GET/PUT /api/v1/auth/config (manage OIDC/local modes)

- Providers:
  
  GET  /api/v1/providers
  
  POST /api/v1/providers
  
  GET  /api/v1/providers/{id}
  
  PUT  /api/v1/providers/{id}
  
  DELETE /api/v1/providers/{id}

- Buckets and objects:
  
  GET  /api/v1/providers/{id}/buckets
  
  POST /api/v1/providers/{id}/buckets
  
  GET  /api/v1/providers/{id}/buckets/{bucket}/objects
  
  POST /api/v1/providers/{id}/buckets/{bucket}/upload  (multipart form: file=@..., key=...)
  
  GET  /api/v1/providers/{id}/buckets/{bucket}/object/{key}
  
  DELETE /api/v1/providers/{id}/buckets/{bucket}/object/{key}

- Observability:
  
  GET /api/v1/metrics
  
  GET /api/v1/errors
  
  GET /api/v1/logs/recent?limit=200

Notes: exact shapes may evolve; inspect internal/api for full details.

### Example: create a MinIO provider

- With Makefile helper (recommended):
  
  make provision-minio

- cURL directly:
  
  curl -sS http://127.0.0.1:8080/api/v1/providers \
    -H 'Content-Type: application/json' \
    -d '{"name":"Local MinIO","type":"minio","endpoint":"127.0.0.1:9000","accessKey":"minioadmin","secretKey":"minioadmin","region":"","useSSL":false}'

---

## Development

Prerequisites: Go 1.24+, podman & podman-compose (for dev stack), jq, curl.

- Run tests:
  
  make test

- Build locally:
  
  make build

- Run locally (uses Postgres from compose by default in Makefile):
  
  make run

- Dev stack lifecycle:
  
  make dev-up
  
  make dev-status
  
  make dev-logs S=minio|postgres
  
  make dev-down

Code layout:

- cmd/server: main entrypoint
- internal/api: routing, handlers (auth, providers, buckets, tracing)
- internal/db: DB init and GORM logger
- internal/logging: structured logger with in‑memory ring + persistence hook
- internal/models: GORM models (users, providers, auth config, logs, traces)
- internal/s3: S3 client wrapper
- web/dist: built SPA assets

---

## Containers and releases

- Dockerfile is multi‑stage and produces a non‑root image
- Multi‑arch images (amd64, arm64) built by GitHub Actions on tags vX.Y.Z
- Registry: ghcr.io/arencloud/hermes with tags: vX.Y.Z, vX.Y, vX, latest
- CI pipelines:
  - Secret leak scan (Gitleaks)
  - Static/vuln analysis (Go vet, govulncheck, Trivy)
  - Release: buildx multi‑arch, push to GHCR, GitHub Release with notes

---

## Security

- Passwords hashed with bcrypt
- Avoid pinning container UID for OpenShift compatibility; runs as non‑root
- CORS is enabled; adjust upstream proxies and headers as needed
- Always rotate the bootstrap admin password on first login

If you find a vulnerability, please open a private security advisory or contact the maintainers.

---

## Roadmap

- Richer UI for browsing and sharing objects
- Fine‑grained permissions and policies
- Signed URLs and lifecycle utilities
- Additional auth options (SAML)
- More metrics/exporters

---

## Contributing

Contributions are welcome! Please open an issue or PR. Run make test before submitting and follow Go formatting conventions.

---

## License

Apache 2.0 — see LICENSE.
