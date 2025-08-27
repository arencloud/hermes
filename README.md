# Hermes

A lightweight, multi‑tenant object storage console and API.

Hermes lets organizations manage users, groups, and roles; connect to S3‑compatible storage (e.g., MinIO, AWS S3); browse, upload, and delete objects; and explore a protected OpenAPI documentation page — all wrapped in a clean Bootstrap UI.

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Docker Image](https://img.shields.io/badge/image-eduard1001171985%2Fhermes%3A1.1.0-2496ED?logo=docker)](https://hub.docker.com/r/eduard1001171985/hermes)
[![Status](https://img.shields.io/badge/status-active-success.svg)](#)


## 📚 Table of Contents
- [Features](#features)
- [Quick Start](#quick-start)
- [First‑time Setup](#first-time-setup)
- [Auth, Roles & Permissions](#auth-roles--permissions)
- [S3/MinIO integration](#s3minio-integration)
- [API](#api)
- [Development](#development)
- [Configuration](#configuration)
- [Logging](#logging)
- [Tooling](#tooling)
- [Deploying on Kubernetes and OpenShift/OKD](#deploying-on-kubernetes-and-openshiftokd)
- [License](#license)
- [Versioning](#versioning)
- [Support & Feedback](#support--feedback)


## ✨ Features
- Organizations, Users, Groups, Roles with assignments (users ↔ roles, groups ↔ roles)
- Fine‑grained permissions via role capabilities: Pull (read) and Push (write)
- Super Admin role grants full access automatically
- S3 storage configuration per organization (endpoint, region, keys, bucket)
- Buckets UI: manage configs, list/browse objects, upload and delete
- Embedded, protected API Docs (/docs) and standalone Swagger UI (/swagger)
- Cookie‑based auth with clean login/signup flows
- Self‑service profile and password management
- JSON REST API with OpenAPI spec


## 📦 Quick Start

You’ll need Docker (or Podman) and Docker Compose for dependencies.

1) Start dependencies (PostgreSQL, MinIO):

```bash
docker compose up -d
# or: podman-compose up -d
```

- Postgres: localhost:5432 (postgres/postgres)
- MinIO: http://localhost:9000 (console: http://localhost:9001)
  - Default creds: minioadmin / minioadmin (from docker-compose.yml)

2) Run Hermes locally (default port: 8080):

Using Makefile:
```bash
make run
```

Or with Go:
```bash
go mod tidy
go build -o bin/hermes ./cmd/hermes
DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres DB_NAME=hermes \
  ./bin/hermes
```

Then open http://localhost:8080


## 🧭 First‑time Setup
- Click "Sign up" to create your first Organization and Admin user.
- The initial user is assigned the system "Super Admin" role, which automatically enables both Pull and Push capabilities.
- After you sign in, you’ll see:
  - Dashboard
  - Buckets (if you have Push)
  - List (if you have Pull)
  - Admin (if you’re Super Admin)
  - API Docs (visible to any user having at least one role)


## 🔐 Auth, Roles & Permissions
- Authentication uses simple secure cookies.
- Role capabilities:
  - Pull: can view buckets, list/download objects
  - Push: can create/update/delete storages and upload/delete objects
- Super Admin: special system role (key: `super_admin`), always grants both Pull and Push and is treated as Admin in the UI.
- Access to API Docs (/docs and /openapi.json via /swagger) is protected. A user must be authenticated and have at least one role to view Swagger.


## ☁️ S3/MinIO integration
- Configure S3 storage per organization under Buckets -> Manage.
- Fields: name, endpoint, region (optional), accessKey, secretKey, useSSL, bucket.
- Browse objects and upload/delete via UI when permitted.


## 🧪 API
- Base path: `/api/v1`
- OpenAPI spec served at `/openapi.json` (protected). UI at:
  - Embedded: `/docs` (inside app layout)
  - Standalone: `/swagger`
- Security scheme: cookieAuth (`auth` cookie)

OpenAPI is kept in `openapi/openapi.json`.


## 🛠️ Development

Prerequisites:
- Go 1.24+ (go.mod sets go 1.24 and toolchain go1.24.6; with GOTOOLCHAIN=auto, Go 1.21–1.23 can auto-download the 1.24 toolchain)

Common tasks:
- Build: `make build`
- Run: `make run`
- Clean: `make clean`
- Container image (Podman): `make image-build` / `make image-push`
- Provision deps (Podman Compose): `make provision` / `make deprovision`

Project layout:
- `cmd/hermes/` – application entrypoint
- `internal/controllers/` – REST controllers
- `internal/routes/` – API and web route wiring
- `internal/models/` – GORM models
- `internal/database/` – DB connection and migrations
- `internal/s3svc/` – S3/MinIO service helpers
- `web/templates/` – Bootstrap UI templates
- `openapi/openapi.json` – API specification


## ⚙️ Configuration
Hermes reads DB connection from environment variables:

- `DB_HOST` (default `localhost`)
- `DB_PORT` (default `5432`)
- `DB_USER` (default `postgres`)
- `DB_PASSWORD` (default `postgres`)
- `DB_NAME` (default `hermes`)
- `DB_SSLMODE` (default `disable`)

At startup, Hermes auto‑migrates the schema for all models.


## 🧾 Logging
Hermes uses structured logging suitable for Kubernetes, OpenShift/OKD, and Podman environments. Logs are emitted to stdout/stderr in JSON by default and include level, timestamp (RFC3339), message, and caller information. HTTP access logs are produced via gin/zap middleware and panics are recovered with stack traces.

Environment variables:
- `HERMES_LOG_FORMAT`: `json` (default) or `console`
- `HERMES_LOG_LEVEL`: `debug`, `info` (default), `warn`, `error`
- `HERMES_LOG_TIME`: `rfc3339` (default) or `epoch`

The deployment config maps set sensible defaults for production (JSON/info). You can override by setting env vars on the Deployment/Pod.


## 🧰 Tooling
- Web framework: Gin
- ORM: GORM (PostgreSQL)
- UI: Bootstrap 5 + Bootstrap Icons
- Object storage: MinIO / S3 compatible
- Secret scanning: Gitleaks in CI (requires GitHub Secret `GITLEAKS_LICENSE`; obtain a license at https://gitleaks.io)


## 📄 License
This project is open source. See the [LICENSE](LICENSE) file for details.


## 🏗️ Deploying on Kubernetes and OpenShift/OKD

Manifests are provided under deploy/ for both generic Kubernetes and OpenShift/OKD.

Important notes:
- Hermes requires a reachable PostgreSQL database. Provide its connection details via environment variables (see Configuration section) using the provided ConfigMap and Secret.
- S3/MinIO configuration is done inside the app UI per organization; no S3 env vars are needed.
- The container listens on port 8080 and exposes /healthz for liveness/readiness probes.

Kubernetes (with kustomize):

1) Prepare DB settings
- Edit deploy/k8s/hermes-config.yaml to set DB_HOST, DB_PORT, DB_USER, DB_NAME, DB_SSLMODE.
- Edit deploy/k8s/hermes-secret.yaml to set DB_PASSWORD (base64-encoded).
- Optionally, deploy the example PostgreSQL stack for testing: kubectl apply -f deploy/k8s/postgres-example.yaml, then set DB_HOST=postgres in the ConfigMap.

2) Deploy Hermes
- kubectl apply -k deploy/k8s
- Optionally configure an Ingress by editing deploy/k8s/ingress.yaml (set host and TLS secret).

OpenShift/OKD (with kustomize):

1) Prepare DB settings
- Edit deploy/openshift/hermes-config.yaml to set DB_* values to your database service.
- Edit deploy/openshift/hermes-secret.yaml to set DB_PASSWORD (base64-encoded).

2) Deploy Hermes and expose with a Route
- oc apply -k deploy/openshift
- This creates a Service and a Route (edge-terminated TLS by default). You can set a custom hostname by editing hermes-route.yaml.

Image
- The manifests reference the published Docker image `eduard1001171985/hermes:v1.1.0`. Adjust the image name/tag to suit your registry, or use kustomize images to override.

Security
- The image runs as non-root; OpenShift will assign an arbitrary UID. The app does not require filesystem write access.

Uninstall
- Kubernetes: kubectl delete -k deploy/k8s
- OpenShift: oc delete -k deploy/openshift


## 🏷️ Versioning
Hermes v1.1.0 — the app name and version appear in the UI title and Swagger.


## 🙋 Support & Feedback
Issues and feature requests are welcome. Please open an issue or a PR.
