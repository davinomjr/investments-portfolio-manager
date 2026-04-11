# B3 Browser Worker Continuation Plan

## Current branch

- `b3-browser-worker-local`

## Goal

- Validate a semi-automatic hosted-style B3 sync flow locally before considering deployment.
- Keep the backend hosted-style flow queue-based.
- Accept that B3 session refresh may still require manual local login.

## What is already implemented

### Backend

- `POST /portfolio/import-b3` now creates an `import_jobs` row and enqueues a `sync_tasks` row instead of directly running the Python worker.
- Internal worker endpoints were added:
  - `POST /internal/sync-tasks/claim`
  - `POST /internal/sync-tasks/{id}/complete`
  - `POST /internal/sync-tasks/{id}/fail`
- `sync_tasks` table was added to SQLite migrations.
- `BROWSER_WORKER_SECRET` was added for internal worker auth.

### Worker

- `python -m app.main run-queue` was added.
- Worker polls the backend over HTTP, claims pending B3 tasks, runs Playwright, and reports completion/failure back to the backend.
- Worker storage paths are now configurable via env:
  - `B3_SESSION_FILE`
  - `B3_DOWNLOAD_DIR`
  - `B3_ARTIFACTS_DIR`
  - `API_BASE_URL`
  - `QUEUE_POLL_INTERVAL_SECONDS`

### Docker Compose

- Added dedicated `browser-worker` service.
- `worker/data` is mounted into the worker container as persistent shared data.

## What has been verified

- `go test ./...` in `backend` passed.
- Python syntax checks passed.
- `docker compose build backend browser-worker` passed.
- `docker compose up -d backend browser-worker frontend` passed.
- End-to-end queue flow worked:
  - frontend request created a B3 import job
  - backend enqueued a sync task
  - worker claimed and processed the task
  - backend job status was updated from worker result

## Latest known result

The latest B3 sync attempt reached the worker, but extraction failed with:

`Could not find a holdings table or downloadable CSV in the B3 portal.`

Database state observed inside the running backend container:

- latest `import_jobs` row: `id=14`, `source='b3'`, `status='failed'`
- latest `sync_tasks` row: `id=1`, `job_id=14`, `provider='b3'`, `status='failed'`

This means the architecture prototype is working. The remaining problem is browser automation reliability inside the container/headless context.

## Important observations

- Host-side manual login saved `worker/data/b3_session.json`.
- The containerized worker could reuse the queue flow, but the B3 page content did not match expectations.
- The failure is no longer “worker not running” or “frontend not enqueueing”; it is now specifically a Playwright/B3 page-state problem.
- `docker compose` showed warnings about an unset variable name like `H2Y2gj`.
  - This strongly suggests a `$` inside one of the env file values.
  - If a password contains `$`, it must be escaped as `$$` in compose env files.

## Next steps

### 1. Add worker failure artifacts

Add artifact capture in `worker/app/extractor.py` and possibly `worker/app/main.py`:

- screenshot on failure
- HTML dump on failure
- current URL on failure
- current document title or a short body excerpt in the error

Write artifacts under `B3_ARTIFACTS_DIR`, for example:

- `artifacts/{timestamp}-{task_id}-failure.png`
- `artifacts/{timestamp}-{task_id}-failure.html`
- `artifacts/{timestamp}-{task_id}-meta.json`

This is the highest priority next step. Without artifacts, debugging B3 failures in container/headless mode is guesswork.

### 2. Improve failure reporting back to backend

Extend worker failure payload and backend storage to include:

- error category
- current URL
- artifact paths

Possible categories:

- `requires_login`
- `challenge_page`
- `selector_changed`
- `download_missing`
- `parse_failed`

### 3. Re-test with the saved session

After artifact capture is in place:

- bootstrap session locally on host again if needed
- trigger `Sync from B3`
- inspect the generated screenshot/HTML

### 4. Decide based on evidence

After seeing the artifact output:

- if the page is a login/challenge/interstitial page, handle that explicitly
- if the page is the correct area but selectors changed, harden the extractor
- if the container/headless rendering differs too much from local, reconsider whether this path is worth pursuing

## Suggested implementation order for the next session

1. Add artifact capture to the worker.
2. Surface artifact/error metadata in logs or backend status.
3. Re-run one B3 sync locally through compose.
4. Inspect screenshot/HTML.
5. Adjust extractor logic only after seeing the actual failed page.

## Useful local commands

Start services:

```bash
docker compose up -d backend browser-worker frontend
```

Bootstrap B3 login locally on host:

```bash
cd worker
python3 -m venv .venv
source .venv/bin/activate
pip install -e .
playwright install chromium
B3_SESSION_FILE="$(pwd)/data/b3_session.json" \
B3_DOWNLOAD_DIR="$(pwd)/data/downloads" \
python -m app.main login
```

Watch worker logs:

```bash
docker compose logs -f browser-worker
```

Inspect DB state:

```bash
docker compose exec backend /bin/sh -lc 'python3 -c "import sqlite3; conn=sqlite3.connect(\"/data/portfolio.db\"); cur=conn.cursor(); print(\"import_jobs\"); [print(row) for row in cur.execute(\"select id, source, status, detail, created_at, updated_at from import_jobs order by id desc limit 10\")]; print(\"sync_tasks\"); [print(row) for row in cur.execute(\"select id, job_id, provider, status, payload, created_at, updated_at from sync_tasks order by id desc limit 10\")]"'
```

## Files most relevant next time

- `backend/internal/services/service.go`
- `backend/internal/httpapi/router.go`
- `backend/internal/db/db.go`
- `docker-compose.yml`
- `worker/app/main.py`
- `worker/app/config.py`
- `worker/app/extractor.py`
