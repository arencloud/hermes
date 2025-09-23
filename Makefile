# Hermes — Developer Makefile (Podman + OCI images)
#
# Goals:
# - Clean, easy dev workflow using podman-compose and OCI images for dependencies
# - Keep the app fast to iterate: run Hermes locally; run MinIO and Postgres in containers
# - One-liners to bring stack up/down and provision a demo provider/bucket
#
# Requirements: podman, podman-compose, curl, jq
# Optional: golang for local build/run (recommended)

SHELL := /bin/bash

# -------- Configurable vars (override via: make run HTTP_PORT=8090) --------
HTTP_PORT ?= 8080
APP_ENV   ?= dev
# DB: use Postgres in containers by default
PGHOST    ?= 127.0.0.1
PGPORT    ?= 5432
PGDATABASE?= hermes
PGUSER    ?= hermes
PGPASSWORD?= hermes
DATABASE_URL ?= postgres://$(PGUSER):$(PGPASSWORD)@$(PGHOST):$(PGPORT)/$(PGDATABASE)?sslmode=disable
DB_DRIVER ?= postgres
# Static assets served from disk (no embed)
STATIC_DIR ?= web/dist

# MinIO
MINIO_ROOT_USER     ?= minioadmin
MINIO_ROOT_PASSWORD ?= minioadmin
MINIO_API_PORT      ?= 9000
MINIO_CONSOLE_PORT  ?= 9001

# Helper
PYC := podman-compose
CURL := curl -sS
JQ := jq -r

# API base (for curl helpers)
API := http://127.0.0.1:$(HTTP_PORT)/api/v1

# Default help target
.PHONY: help
help:
	@echo "Hermes Developer Makefile (Podman)"
	@echo
	@echo "Common targets:"
	@echo "  make dev-up            # Start Postgres + MinIO via podman-compose"
	@echo "  make dev-down          # Stop and remove containers"
	@echo "  make dev-logs          # Tail container logs"
	@echo "  make run               # Run Hermes locally (uses Postgres & StaticDir)"
	@echo "  make wait-server       # Wait until Hermes /health is up"
	@echo "  make provision-minio   # Create a 'Local MinIO' provider via API"
	@echo "  make demo-bucket       # Create demo bucket via API"
	@echo "  make demo-upload       # Upload sample object via API"
	@echo
	@echo "Config vars (override on command line): HTTP_PORT=$(HTTP_PORT) DB_DRIVER=$(DB_DRIVER) DATABASE_URL=$(DATABASE_URL) STATIC_DIR=$(STATIC_DIR)"

# -------- Dev stack (Podman Compose) --------
.PHONY: dev-up dev-down dev-logs dev-status

dev-up:
	MINIO_ROOT_USER=$(MINIO_ROOT_USER) MINIO_ROOT_PASSWORD=$(MINIO_ROOT_PASSWORD) \
	PGDATABASE=$(PGDATABASE) PGUSER=$(PGUSER) PGPASSWORD=$(PGPASSWORD) \
	$(PYC) up -d
	@echo "MinIO:    http://127.0.0.1:$(MINIO_API_PORT) (console: http://127.0.0.1:$(MINIO_CONSOLE_PORT))"
	@echo "Postgres: $(PGHOST):$(PGPORT)/$(PGDATABASE) user=$(PGUSER)"

# Gracefully stop and remove containers
dev-down:
	$(PYC) down

# Tail both logs
# Use: make dev-logs S=minio  (or S=postgres)
dev-logs:
	@if [ "$(S)" = "minio" ]; then podman logs -f hermes-minio; \
	elif [ "$(S)" = "postgres" ]; then podman logs -f hermes-postgres; \
	else echo "Set S=minio or S=postgres"; fi

# Quick status info
dev-status:
	@podman ps --filter name=hermes-
	@echo "MinIO health: $$($(CURL) -o /dev/null -w '%{http_code}' http://127.0.0.1:$(MINIO_API_PORT)/minio/health/ready || true)"

# -------- App build/run/test (local) --------
.PHONY: build test run clean

# Build metadata from git (fallbacks)
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

build:
	@echo "Building Hermes… (version=$(VERSION) commit=$(COMMIT))"
	GOFLAGS= go build -ldflags "-s -w -X github.com/arencloud/hermes/internal/version.Version=$(VERSION) -X github.com/arencloud/hermes/internal/version.Commit=$(COMMIT) -X github.com/arencloud/hermes/internal/version.BuildDate=$(BUILD_DATE)" -o server ./cmd/server

run: build
	APP_ENV=$(APP_ENV) HTTP_PORT=$(HTTP_PORT) \
	DB_DRIVER=$(DB_DRIVER) DATABASE_URL='$(DATABASE_URL)' \
	STATIC_DIR=$(STATIC_DIR) \
	./server

test:
	go test ./internal/... ./cmd/...

clean:
	rm -f server

# -------- Helpers: wait and provision via API --------
.PHONY: wait-server wait-minio provision-minio demo-bucket demo-upload

wait-server:
	@echo -n "Waiting for Hermes at http://127.0.0.1:$(HTTP_PORT)/health "; \
	for i in $$(seq 1 60); do \
		code=$$($(CURL) -o /dev/null -w '%{http_code}' http://127.0.0.1:$(HTTP_PORT)/health || true); \
		if [ "$$code" = "200" ]; then echo "OK"; exit 0; fi; \
		echo -n "."; sleep 1; \
	done; echo " timeout"; exit 1

wait-minio:
	@echo -n "Waiting for MinIO at http://127.0.0.1:$(MINIO_API_PORT) "; \
	for i in $$(seq 1 60); do \
		code=$$($(CURL) -o /dev/null -w '%{http_code}' http://127.0.0.1:$(MINIO_API_PORT)/minio/health/ready || true); \
		if [[ "$$code" =~ ^(200|405)$$ ]]; then echo "OK"; exit 0; fi; \
		echo -n "."; sleep 1; \
	done; echo " timeout"; exit 1

# Create or find a provider that points to local MinIO
provision-minio: wait-server wait-minio
	@echo "Provisioning Local MinIO provider via Hermes API…"
	@set -e; \
	BODY=$$(jq -n --arg name "Local MinIO" \
		--arg typ "minio" \
		--arg endpoint "127.0.0.1:$(MINIO_API_PORT)" \
		--arg ak "$(MINIO_ROOT_USER)" \
		--arg sk "$(MINIO_ROOT_PASSWORD)" \
		--arg region "" \
		--argjson useSSL false \
		'{name:$$name, type:$$typ, endpoint:$$endpoint, accessKey:$$ak, secretKey:$$sk, region:$$region, useSSL:$$useSSL}'); \
	EXIST=$$($(CURL) $(API)/providers | $(JQ) '.[] | select(.name=="Local MinIO") | .id' | head -n1 || true); \
	if [ -n "$$EXIST" ] && [ "$$EXIST" != "null" ]; then echo "Provider exists (id=$$EXIST)"; exit 0; fi; \
	RESP=$$($(CURL) -w "\n%{http_code}" -H 'Content-Type: application/json' -d "$$BODY" $(API)/providers); \
	CODE=$$(echo "$$RESP" | tail -n1); BODY=$$(echo "$$RESP" | head -n-1); \
	if [ "$$CODE" != "201" ] && [ "$$CODE" != "200" ]; then echo "Failed (HTTP $$CODE): $$BODY"; exit 1; fi; \
	echo "Created provider: $$BODY"

# Create a demo bucket on the Local MinIO provider
DEMO_BUCKET ?= demo-bucket

demo-bucket: wait-server
	@set -e; \
	PID=$$($(CURL) $(API)/providers | $(JQ) '.[] | select(.name=="Local MinIO") | .id' | head -n1); \
	if [ -z "$$PID" ] || [ "$$PID" = "null" ]; then echo "Local MinIO provider not found. Run: make provision-minio"; exit 1; fi; \
	BODY=$$(jq -n --arg name "$(DEMO_BUCKET)" --arg region "" '{name:$$name, region:$$region}'); \
	RESP=$$($(CURL) -w "\n%{http_code}" -H 'Content-Type: application/json' -d "$$BODY" $(API)/providers/$$PID/buckets); \
	CODE=$$(echo "$$RESP" | tail -n1); \
	if [ "$$CODE" != "201" ] && [ "$$CODE" != "200" ]; then echo "Create bucket failed (HTTP $$CODE): $$(echo "$$RESP" | head -n-1)"; exit 1; fi; \
	echo "Bucket $(DEMO_BUCKET) ready"

# Upload a small sample file using the API
DEMO_FILE ?= /tmp/hermes-demo.txt

demo-upload: wait-server
	@echo "Hello from Hermes $$(date -u)" > $(DEMO_FILE)
	@set -e; \
	PID=$$($(CURL) $(API)/providers | $(JQ) '.[] | select(.name=="Local MinIO") | .id' | head -n1); \
	if [ -z "$$PID" ] || [ "$$PID" = "null" ]; then echo "Local MinIO provider not found. Run: make provision-minio"; exit 1; fi; \
	BUCKET=$(DEMO_BUCKET); KEY=$$(basename $(DEMO_FILE)); \
	RESP=$$($(CURL) -w "\n%{http_code}" -F file=@$(DEMO_FILE) -F key=$$KEY $(API)/providers/$$PID/buckets/$$BUCKET/upload); \
	CODE=$$(echo "$$RESP" | tail -n1); \
	if [ "$$CODE" != "200" ]; then echo "Upload failed (HTTP $$CODE): $$(echo "$$RESP" | head -n-1)"; exit 1; fi; \
	echo "Uploaded $(DEMO_FILE) to $$BUCKET/$$KEY"

# -------- Quality of life --------
.PHONY: env print-db-url

env:
	@echo "APP_ENV=$(APP_ENV)"
	@echo "HTTP_PORT=$(HTTP_PORT)"
	@echo "DB_DRIVER=$(DB_DRIVER)"
	@echo "DATABASE_URL=$(DATABASE_URL)"
	@echo "STATIC_DIR=$(STATIC_DIR)"
	@echo "MINIO: http://127.0.0.1:$(MINIO_API_PORT) (console http://127.0.0.1:$(MINIO_CONSOLE_PORT))"

print-db-url:
	@echo $(DATABASE_URL)
