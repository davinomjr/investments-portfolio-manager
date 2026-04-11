from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

from app.config import config
from app.extractor import B3PortfolioExtractor, SessionExpiredError
from app.ibkr_extractor import IbkrExtractor


def _api_request(method: str, path: str, payload: dict | None = None) -> dict | None:
    data = None
    headers = {"Content-Type": "application/json"}
    if config.browser_worker_secret:
        headers["X-Worker-Secret"] = config.browser_worker_secret
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=True).encode("utf-8")
    request = urllib.request.Request(
        f"{config.api_base_url.rstrip('/')}{path}",
        data=data,
        headers=headers,
        method=method,
    )
    with urllib.request.urlopen(request, timeout=60) as response:
        body = response.read().decode("utf-8").strip()
    if not body:
        return None
    return json.loads(body)


def _claim_task() -> dict | None:
    payload = _api_request("POST", "/internal/sync-tasks/claim", {"provider": "b3"})
    if not payload:
        return None
    return payload.get("task")


def _complete_task(task_id: int, payload: dict) -> None:
    _api_request("POST", f"/internal/sync-tasks/{task_id}/complete", payload)


def _fail_task(task_id: int, status: str, detail: str) -> None:
    _api_request("POST", f"/internal/sync-tasks/{task_id}/fail", {"status": status, "detail": detail})


def run_queue(*, once: bool) -> int:
    config.download_dir.mkdir(parents=True, exist_ok=True)
    config.session_file.parent.mkdir(parents=True, exist_ok=True)
    config.artifacts_dir.mkdir(parents=True, exist_ok=True)

    while True:
        try:
            task = _claim_task()
        except urllib.error.URLError as exc:
            print(f"queue poll failed: {exc}", file=sys.stderr)
            if once:
                return 1
            time.sleep(config.queue_poll_interval_seconds)
            continue

        if not task:
            if once:
                return 0
            time.sleep(config.queue_poll_interval_seconds)
            continue

        extractor = B3PortfolioExtractor()
        try:
            result = extractor.import_portfolio().model_dump()
            detail = f"Imported {len(result.get('holdings', []))} positions from B3"
            _complete_task(task["id"], {"holdings": result.get("holdings", []), "detail": detail})
        except SessionExpiredError as exc:
            _fail_task(task["id"], "requires_login", str(exc))
        except Exception as exc:
            _fail_task(task["id"], "failed", str(exc))

        if once:
            return 0


def main() -> int:
    parser = argparse.ArgumentParser(description="B3 portfolio import worker")
    subparsers = parser.add_subparsers(dest="command", required=True)

    login_parser = subparsers.add_parser("login", help="Open browser for manual login and save session")
    login_parser.add_argument("--json", action="store_true")

    import_parser = subparsers.add_parser("import", help="Import portfolio from B3")
    import_parser.add_argument("--json", action="store_true")

    manual_parser = subparsers.add_parser("import-file", help="Import portfolio from a manually exported B3 file")
    manual_parser.add_argument("path")
    manual_parser.add_argument("--json", action="store_true")

    ibkr_parser = subparsers.add_parser("import-ibkr", help="Import portfolio from IBKR Flex Web Service")
    ibkr_parser.add_argument("--json", action="store_true")

    queue_parser = subparsers.add_parser("run-queue", help="Poll backend for pending B3 sync tasks")
    queue_parser.add_argument("--once", action="store_true")

    args = parser.parse_args()

    if args.command == "run-queue":
        return run_queue(once=args.once)

    try:
        if args.command == "import-ibkr":
            payload = IbkrExtractor().import_portfolio().model_dump()
        else:
            extractor = B3PortfolioExtractor()
            if args.command == "login":
                session_path = extractor.bootstrap_login()
                payload = {"status": "ok", "session_file": str(session_path)}
            elif args.command == "import-file":
                payload = extractor.import_manual_file(Path(args.path)).model_dump()
            else:
                payload = extractor.import_portfolio().model_dump()
    except SessionExpiredError as exc:
        print(str(exc), file=sys.stderr)
        return 2
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        return 1

    if getattr(args, "json", False):
        print(json.dumps(payload, ensure_ascii=True))
    else:
        print(payload)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
