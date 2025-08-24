# Makefile for Hermes
# Usage examples:
#   make up               # build image and start via podman-compose
#   make down             # stop and remove containers
#   make logs             # tail logs
#   make build            # build Go binary locally
#   make run              # run local binary on PORT
#   make health           # check container health endpoint
#   make open             # open app in browser

# --- Config ---
PROJECT_NAME := hermes
COMPOSE := podman-compose
PORT ?= 8080
HERMES_SESSION_SECRET ?= dev-secret-change-me
BROWSER ?= xdg-open

# Default target
.PHONY: help
help:
	@echo "Targets:"
	@echo "  make up        - Build image and start containers (detached)"
	@echo "  make down      - Stop and remove containers"
	@echo "  make restart   - Restart containers"
	@echo "  make logs      - Tail service logs"
	@echo "  make ps        - Show compose services"
	@echo "  make health    - Curl /healthz from host"
	@echo "  make open      - Open http://localhost:$(PORT)"
	@echo "  make build     - Go build local binary to bin/"
	@echo "  make run       - Run local binary with PORT=$(PORT)"
	@echo "  make tidy      - go mod tidy"
	@echo "  make clean     - Compose down with volumes"

# --- Local dev ---
.PHONY: build
build:
	@echo "Building Go binary..."
	go build -o bin/$(PROJECT_NAME) ./cmd/$(PROJECT_NAME)

.PHONY: run
run: build
	@echo "Running local binary on :$(PORT)"
	PORT=$(PORT) ./bin/$(PROJECT_NAME)

.PHONY: tidy
tidy:
	go mod tidy

# --- Container orchestration (podman-compose) ---
.PHONY: up
yup:  ## alias for up (common typo)
up:
	HERMES_SESSION_SECRET=$(HERMES_SESSION_SECRET) PORT=$(PORT) $(COMPOSE) up -d --build

.PHONY: down
down:
	$(COMPOSE) down

.PHONY: clean
clean:
	$(COMPOSE) down -v

.PHONY: restart
restart:
	$(COMPOSE) restart || ( $(COMPOSE) up -d )

.PHONY: logs
logs:
	podman logs -f $(PROJECT_NAME)

.PHONY: ps
ps:
	$(COMPOSE) ps

.PHONY: health
health:
	curl -sf http://127.0.0.1:$(PORT)/healthz && echo ok || (echo "health check failed" && exit 1)

.PHONY: open
open:
	$(BROWSER) http://localhost:$(PORT) >/dev/null 2>&1 || true
