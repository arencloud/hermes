Hermes

Hermes is a lightweight web application for exploring a multi-tenant object storage concept with a simple UI, role-based access control, and optional PostgreSQL-backed user management. It provides server-rendered pages for common workflows (login, dashboard, tenants, buckets, uploads, users, profile) and a small JSON API for authentication and user administration.

Key features
- Login/logout with session-based authentication
- Built-in default admin user on first run
- Role-based pages (admin-only Users section)
- Optional PostgreSQL persistence for users (fallback to in-memory store)
- Health endpoint for orchestration (/healthz)

Project layout
- cmd/hermes/main.go — web server and routes (Gin)
- internal/auth — auth store, middleware, optional PostgreSQL integration
- templates/ — server-rendered HTML templates
- Containerfile — multi-stage container build
- docker-compose.yml — app + PostgreSQL for local dev
- Makefile — convenience targets for dev and ops

Requirements
- Go 1.22+ (for local builds)
- Podman or Docker (for containers); podman-compose or docker-compose for orchestration

Configuration (environment variables)
- PORT: Web server port (default: 8080)
- HERMES_SESSION_SECRET: Cookie/session secret (default: dev-secret-change-me; change in production)
- DATABASE_URL: PostgreSQL DSN. If set, Hermes stores users in Postgres; if empty, uses in-memory store
  Example: postgres://postgres:postgres@localhost:5432/hermes?sslmode=disable

Health check
- GET /healthz returns "ok" when the app is running

Quick start
Option A — Run locally (no DB, in-memory users)
1) Build and run
   make run
   # or:
   go build -o bin/hermes ./cmd/hermes && PORT=8080 ./bin/hermes
2) Open http://localhost:8080

Option B — Containers with PostgreSQL (recommended for full features)
1) Start with compose (builds the image, starts Postgres and Hermes)
   make up
   # or if you use docker-compose instead of podman-compose:
   COMPOSE=docker-compose make up
2) Open http://localhost:8080
3) Tail logs (optional)
   make logs
4) Stop
   make down

Option C — Build and run the container directly
- Build image
  podman build -t hermes:dev -f Containerfile .
  # or: docker build -t hermes:dev -f Containerfile .
- Run (without DB, in-memory users)
  podman run --rm -p 8080:8080 -e PORT=8080 -e HERMES_SESSION_SECRET=dev-secret-change-me hermes:dev

Default credentials
- Username: admin
- Password: password
Note: The default admin is created at startup. Change the password immediately for any non-development use.

How to use Hermes
1) Login
   - Visit http://localhost:8080
   - You will be redirected to /login
   - Enter: admin / password (default)
2) Dashboard
   - After login, you’ll land on the Dashboard with placeholder stats
3) Buckets
   - Navigate to Buckets to see a placeholder list
   - Click a bucket to view details (mock) and the Upload page link
4) Tenants
   - Tenants page displays a placeholder list of tenants
5) Users (admin-only)
   - Navigate to Users to view current users
   - Create, update, and delete users via the UI or JSON API (see below)
6) Profile
   - View basic profile information under Profile
7) Logout
   - Use the logout link to end your session

JSON API (admin where indicated)
- POST /api/v1/auth/login
  Body: {"tenant":"default","username":"admin","password":"password"}
  Response: {"access_token":"dummy-access-token","refresh_token":"dummy-refresh-token"}
  Note: Tokens are placeholders; sessions are used for the server-rendered UI.

- GET /api/v1/users (admin)
  Response: {"users":[{"username":"...","role":"...","tenant":"..."}, ...]}

- POST /api/v1/users (admin)
  Body: {"username":"u","password":"p","role":"user|developer|admin","tenant":"t"}

- PUT /api/v1/users/:username (admin)
  Body: {"role":"...","tenant":"...","password":"optional-new-password"}

- DELETE /api/v1/users/:username (admin)

Security notes
- Always set a strong HERMES_SESSION_SECRET in production
- Change the default admin password immediately
- Use HTTPS and secure cookie settings in real deployments (reverse proxy/ingress recommended)

Next steps (how to run and how to use)
1) Choose your run mode
   - Quick test: make run (in-memory users)
   - Full local stack: make up (with PostgreSQL)
   - Container-only: podman run -p 8080:8080 hermes:dev
2) Open the app
   - Visit http://localhost:8080 (or your mapped PORT)
3) Login
   - Use admin / password (change immediately after login)
4) Explore the UI
   - Dashboard: Overview
   - Users: Create a non-admin user for daily use
   - Tenants and Buckets: Explore pages and flows
   - Upload: Visit any bucket’s Upload page (mock UI)
5) Persist users (optional)
   - Ensure DATABASE_URL is set (via docker-compose or env var) to enable PostgreSQL-backed users
6) Operational checks
   - Health: curl -fsS http://localhost:${PORT:-8080}/healthz
   - Logs: make logs (when using compose)

Troubleshooting
- Build behind proxy: set build args GOPROXY and GOSUMDB via docker-compose or podman build
- Permission issues during container build: this repo uses a writable WORKDIR and builds into ./out/ in the builder stage
- Postgres connectivity: verify DATABASE_URL, container health, and that db service is healthy before Hermes starts
