# CLAUDE.md — Investments Portfolio Manager

This document provides guidance for AI assistants working in this repository.

---

## Project Overview

A full-stack **Brazilian stock portfolio management application** that imports holdings from the B3 investor portal, shows allocation charts, runs Monte Carlo simulations, and surfaces quarterly earnings data.

**Tech stack:**

| Layer     | Technology                              |
|-----------|-----------------------------------------|
| Backend   | Go 1.24, pure-Go SQLite (`modernc.org/sqlite`) |
| Frontend  | Next.js 15, React 19, Tailwind CSS 3, TypeScript 5 |
| Worker    | Python + Playwright (B3 browser automation) |
| Database  | SQLite (WAL mode, single file `database/portfolio.db`) |
| CI/CD     | GitHub Actions (issue summarization only) |

---

## Repository Layout

```
investments-portfolio-manager/
├── backend/                  # Go REST API
│   ├── cmd/api/main.go       # Entry point
│   └── internal/
│       ├── config/config.go  # Env-based config loading
│       ├── db/db.go          # SQLite open + schema migration
│       ├── httpapi/router.go # Route registration & JSON handlers
│       ├── models/models.go  # Shared response structs
│       └── services/
│           ├── service.go          # Core business logic (~1000 lines)
│           ├── sentiment.go        # Sentiment analysis (~855 lines)
│           ├── sentiment_test.go   # Sentiment unit tests
│           └── service_montecarlo_test.go
├── frontend/                 # Next.js dashboard
│   ├── app/
│   │   ├── layout.tsx        # Root layout + TopNav
│   │   ├── page.tsx          # Portfolio dashboard (SSR)
│   │   ├── results/page.tsx  # Quarterly results view
│   │   └── simulator/page.tsx
│   ├── components/           # All UI components
│   └── lib/
│       ├── api.ts            # Typed fetch wrappers for all API calls
│       └── asset-style.ts    # Asset-type colour/icon constants
├── worker/                   # Python Playwright worker
│   └── app/
│       ├── main.py           # CLI entry point (login / import / import-file)
│       ├── extractor.py      # B3 portal automation
│       ├── parser.py         # XLSX/CSV parsing & normalization
│       ├── models.py         # Holding, ImportResult dataclasses
│       └── config.py         # Worker configuration
├── backend-python/           # Legacy Python backend (reference only — do not modify)
├── database/
│   └── init.sql              # Reference PostgreSQL DDL (schema is auto-migrated in Go)
├── simulator/                # Reserved — Monte Carlo engine (future)
├── Makefile                  # Developer convenience targets
└── README.md                 # User-facing project overview
```

---

## Development Workflows

### Running the full stack locally

```bash
# Backend (Go) — listens on 127.0.0.1:8000
make run backend

# Frontend (Next.js) — listens on localhost:3000
make run frontend

# Worker (one-off commands)
cd worker
python -m venv .venv && source .venv/bin/activate
pip install -e .
python -m app.main login        # open browser for manual B3 login
python -m app.main import       # reuse stored session, sync positions
python -m app.main import-file /path/to/posicao.xlsx --json
```

### Setup from scratch

```bash
make setup-backend    # go mod tidy
make setup-frontend   # npm install (inside frontend/)
```

### Running tests

```bash
# Backend unit tests
cd backend && go test ./internal/services/...

# No frontend or worker tests exist yet
```

### Building

There is no explicit build step for development. For production:
- Backend: `go build ./cmd/api` → produces a single binary
- Frontend: `npm run build` inside `frontend/`

---

## Key Conventions

### Go (backend)

- **Package layout follows clean architecture**: config → db → models → services → httpapi. Layers only import downward.
- **All handlers live in `httpapi/router.go`**. A handler calls a `Service` method and writes JSON.
- **Service struct** (`internal/services/service.go`) is the only layer that touches the database. No raw SQL outside `service.go` or `db.go`.
- **Error responses** always use `{"detail": "message"}` with an appropriate HTTP status code.
- **Async jobs**: POST endpoints that trigger long-running work return `202 Accepted` with a job ID immediately; clients poll `GET /portfolio/import-jobs/latest`.
- **CORS**: The CORS middleware (`router.go`) permits `http://localhost:3000` only. Update this if the frontend port changes.
- **Configuration via environment variables** (see `internal/config/config.go`):
  - `ADDR` (default `127.0.0.1:8000`)
  - `DATABASE_URL` (default `../database/portfolio.db`)
  - `WORKER_DIR`, `WORKER_PYTHON`, `WORKER_MODULE`, `WORKER_COMMAND`
  - `UPLOAD_DIR`, `DATA_CACHE_DIR`
  - `CVM_ITR_BASE_URL`
  - `SENTIMENT_*` flags (enable/disable and tune sentiment feature)
- **SQLite only** (`modernc.org/sqlite` — pure Go, no CGO). Do not introduce CGO or Postgres dependencies without discussion.
- **Schema changes**: Add new `CREATE TABLE IF NOT EXISTS` or `ALTER TABLE` statements inside the migration function in `internal/db/db.go`. There is no external migration tool.
- **Test style**: Table-driven tests, mock HTTP servers via `net/http/httptest`. See `sentiment_test.go` for the established pattern.

### TypeScript / Next.js (frontend)

- **Data fetching is server-side** in `page.tsx` files using `async` components and `Promise.all()`.
- **Client components** (interactive UI) use `"use client"` and are placed in `components/`.
- **All API calls go through `lib/api.ts`**. Never call `fetch()` directly in a component; add a typed wrapper there instead.
- **Path alias**: `@/*` maps to the project root (configured in `tsconfig.json`).
- **Styling**: Tailwind utility classes with a custom palette — use the design tokens:
  - `ink` — primary dark background
  - `sand` — secondary/muted backgrounds
  - `clay` — accent / hover states
  - `pine` — success / positive
  - `gold` — warning / neutral highlight
  - Font: serif (Georgia) via Tailwind config
- **Dark theme throughout** — do not introduce light-mode-only colours.
- **No test suite exists yet** for the frontend. When adding tests, use Jest + React Testing Library (already installed as dev deps).
- **API proxy**: `next.config.ts` rewrites `/api/*` to the backend at `INTERNAL_API_BASE_URL`. Set `NEXT_PUBLIC_API_BASE_URL=/api` in `.env.local`.

### Python (worker)

- **Playwright with persistent session state** stored in `worker/data/b3_session.json`. Always attempt session reuse before prompting for login.
- **CSV-first parsing**: prefer programmatic CSV export from B3; fall back to HTML table scraping only if CSV is unavailable.
- **Configuration via environment variables**: `B3_CPF`, `B3_PASSWORD`.
- **CLI subcommands**: `login`, `import`, `import-file`. Keep the CLI interface stable; the backend invokes the worker as a subprocess.
- **Output contract**: the worker writes JSON to stdout when called with `--json`. The Go backend reads this output — any change to the JSON schema must be reflected in `models.go`.

---

## Database Schema (SQLite)

Managed automatically by Go migrations in `internal/db/db.go`. Tables:

| Table | Purpose |
|-------|---------|
| `assets` | Ticker, asset type (stock/FII/ETF…), currency |
| `positions` | Holdings: quantity, average price, broker, import source |
| `asset_metadata` | Company name, tax ID (CNPJ) |
| `import_jobs` | Async job tracking (status: pending/running/done/error) |
| `sentiment_snapshots` | Cached sentiment scores with TTL |
| `sentiment_sources` | Individual data points feeding a snapshot |
| `sentiment_refresh_log` | Audit trail for sentiment refreshes |

Reference DDL for the original PostgreSQL design is in `database/init.sql` (not used at runtime).

---

## API Endpoints (backend)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/portfolio` | Aggregated portfolio summary |
| `GET` | `/positions` | All current positions |
| `GET` | `/portfolio/import-jobs/latest` | Latest import job status |
| `POST` | `/portfolio/import/b3` | Trigger async B3 import (202) |
| `POST` | `/portfolio/import/file` | Manual file upload import (202) |
| `GET` | `/portfolio/monte-carlo` | Run Monte Carlo simulation |
| `GET` | `/portfolio/sentiment` | Sentiment analysis snapshot |
| `GET` | `/portfolio/results` | Quarterly earnings results |

---

## Important Files to Know

| File | Why it matters |
|------|---------------|
| `backend/internal/services/service.go` | Core logic for all features; start here for backend changes |
| `backend/internal/httpapi/router.go` | All HTTP routes and request/response wiring |
| `backend/internal/db/db.go` | Schema migrations; edit here to add tables/columns |
| `frontend/lib/api.ts` | Single source of truth for frontend↔backend API contract |
| `frontend/app/page.tsx` | Main dashboard — server-side data fetching entry point |
| `worker/app/extractor.py` | B3 browser automation; fragile — test carefully after changes |
| `worker/app/parser.py` | XLSX/CSV parsing; must handle B3 format variations |
| `Makefile` | Developer scripts — check here before writing new shell commands |

---

## What to Avoid

- **Do not modify `backend-python/`** — this is legacy reference code only.
- **Do not introduce CGO** in the Go backend; the SQLite driver is pure Go by design.
- **Do not hard-code localhost URLs** in source files; use environment variables or the Next.js proxy rewrite.
- **Do not commit secrets**: `.gitignore` excludes `.env`, `worker/.env`, `frontend/.env.local`, session files, and the SQLite database. Never add these files to the repository.
- **Do not change the worker's stdout JSON schema** without updating `backend/internal/models/models.go` and the parsing code in `service.go`.
- **Do not add light-mode styles** without confirming the design direction — the UI is intentionally dark-themed.

---

## CI/CD

The only GitHub Actions workflow (`.github/workflows/summary.yml`) auto-summarizes new issues using GitHub's AI inference action. It does **not** run tests or deploy anything. There is no automated test or deployment pipeline at this time.

---

## Branching & Commit Guidelines

- Feature branches follow the pattern `claude/<short-description>-<id>` or `feature/<short-description>`.
- Commit messages are imperative and lowercase (e.g. `add monte carlo endpoint`, `fix: sentiment ttl check`).
- Keep commits focused; one logical change per commit.
