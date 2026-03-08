# backend

Primary Go backend for the project. It reuses the same local SQLite database and Python worker.

## What It Looks Like

The backend keeps the same split as the old Python service, but in Go:

- `cmd/api`
  - process entrypoint and HTTP server bootstrap
- `internal/config`
  - env-based runtime configuration
- `internal/db`
  - SQLite connection and schema migration
- `internal/httpapi`
  - route registration, JSON responses, CORS
- `internal/services`
  - portfolio import orchestration, DB reads, CVM quarterly results
- `internal/models`
  - transport structs returned to the frontend

The intent is:

- same endpoints as the historical Python backend
- same database file as the historical Python backend
- same Python worker for `import-b3` and `import-file`
- default port `8000`, so the frontend can treat it as the primary API

## Endpoints

- `GET /portfolio`
- `GET /positions`
- `POST /portfolio/import-b3`
- `POST /portfolio/import-file`
- `GET /stocks/latest-results`

## Defaults

- address: `http://127.0.0.1:8000`
- database: `../database/portfolio.db`
- worker directory: `../worker`
- worker command: auto-detected project Python, then `python3 -m app.main`

## Run

From the repo root:

```bash
make run backend
```

Or directly:

```bash
cd backend
go mod tidy
go run ./cmd/api
```

## Test It With The Frontend

Point the Next.js app at the Go backend:

```bash
cd frontend
cat <<'EOF' > .env.local
NEXT_PUBLIC_API_BASE_URL=/api
INTERNAL_API_BASE_URL=http://127.0.0.1:8000
API_PROXY_TARGET=http://127.0.0.1:8000
EOF
npm run dev
```

Then open:

- `http://localhost:3000`

## Current Limitation

This package depends on `modernc.org/sqlite`. In an environment without outbound Go module access, `go mod tidy` cannot fetch it. On a normal local machine with network access, `make run backend` should download the module and start the API.
