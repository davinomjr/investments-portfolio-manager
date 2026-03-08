from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from app.extractor import B3PortfolioExtractor, SessionExpiredError


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

    args = parser.parse_args()
    extractor = B3PortfolioExtractor()

    try:
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
