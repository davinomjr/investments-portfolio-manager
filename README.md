# Portfolio Manager

Current structure:

- `frontend/`: Next.js dashboard for portfolio import, holdings review, and quarterly results
- `backend/`: primary Go API
- `backend-python/`: historical FastAPI backend kept for reference
- `worker/`: Python worker for B3 import and manual workbook parsing
- `database/`: database bootstrap artifacts
- `simulator/`: reserved for the upcoming scenario and Monte Carlo engine

## Current scope

Implemented now:

- B3 import job endpoint: `POST /portfolio/import-b3`
- manual file import endpoint: `POST /portfolio/import-file`
- Portfolio read endpoints: `GET /portfolio`, `GET /positions`
- latest quarterly stock results endpoint: `GET /stocks/latest-results`
- Playwright worker with:
  - storage-state session reuse
  - CSV-first extraction
  - HTML table fallback
  - normalized holding output
  - expired-session detection

Planned next:

- scenario engine
- Monte Carlo simulator
- richer portfolio analysis workflows

## Run the Go backend

```bash
cd /path/to/portfolio-manager
make run backend
```

The API runs on `http://127.0.0.1:8000` by default.

## Run the frontend

```bash
cd /path/to/portfolio-manager
make run frontend
```

The frontend uses a local `/api` proxy to the Go backend by default.

## Run the worker login bootstrap

```bash
cd worker
python -m venv .venv
source .venv/bin/activate
pip install -e .
python -m app.main login
```

This opens a Playwright browser for manual B3 login. When login succeeds, the storage state is saved to `worker/data/b3_session.json`.

## Run worker import directly

```bash
cd worker
python -m app.main import
```

## Run manual import from B3 export file

```bash
cd worker
python -m app.main import-file /absolute/path/to/posicao.xlsx --json
```

This accepts `.xlsx` or `.csv` exports and normalizes the holdings without opening Playwright.

## Backend-triggered import

Set `WORKER_COMMAND` or `WORKER_PYTHON` if you want the Go backend to call the worker through a custom Python executable or wrapper.

## Quarterly Results Setup

The quarterly results panel now uses official CVM `ITR` quarterly filings.

Notes:

- no paid API key is required
- the backend downloads the latest `ITR` zip from CVM and caches it locally
- after pulling this change, re-upload your B3 workbook once so issuer metadata is stored for CVM matching

## Legacy Python backend

```bash
cd /path/to/portfolio-manager
make run backend-python
```

This is kept only for reference and comparison. The main app should use the Go backend under `backend/`.
