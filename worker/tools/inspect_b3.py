"""Read-only diagnostic: show what parse_b3_xlsx sees in a B3 position export.

Usage (from the worker/ directory, with the venv active):

    python tools/inspect_b3.py                 # uses data/downloads/portfolio.xlsx
    python tools/inspect_b3.py path/to/file.xlsx
    python tools/inspect_b3.py path/to/file.xlsx BBAS3

For each sheet it prints the header, the detected ticker/quantity columns, and
every row matching the ticker(s) of interest (default: all). Nothing is
written or uploaded — this just helps us see where a quantity comes from.
"""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from app.parser import (  # noqa: E402
    _is_holdings_sheet,
    _pick,
    _read_xlsx_workbook,
    _row_value,
    normalize_ticker,
)


def main() -> None:
    args = [a for a in sys.argv[1:]]
    default = Path(__file__).resolve().parents[1] / "data" / "downloads" / "portfolio.xlsx"
    path = Path(args[0]) if args and args[0].lower().endswith((".xlsx", ".xlsm")) else default
    wanted = {normalize_ticker(a) for a in args if not a.lower().endswith((".xlsx", ".xlsm"))}

    if not path.exists():
        print(f"File not found: {path}")
        print("Pass the path explicitly: python tools/inspect_b3.py /path/to/portfolio.xlsx")
        return

    print(f"Reading: {path}\n")
    workbook = _read_xlsx_workbook(path)
    print(f"Sheets ({len(workbook)}): {list(workbook.keys())}\n")

    for sheet_name, rows in workbook.items():
        normalized = sheet_name.strip().lower()
        counted = _is_holdings_sheet(normalized)
        flag = "COUNTED" if counted else "SKIPPED (informational)"
        print(f"=== {sheet_name!r}  ->  {flag}")
        if not rows:
            print("   (empty)\n")
            continue

        header = rows[0]
        columns = {str(c).lower().strip(): i for i, c in enumerate(header) if str(c).strip()}
        ticker_col = _pick(columns, ["código de negociação", "codigo de negociacao", "ticker", "código", "codigo"])
        qty_col = _pick(columns, ["quantidade disponível", "quantidade disponivel", "quantidade", "quantity", "qtd"])
        prod_col = _pick(columns, ["produto", "ativo", "asset"])
        print(f"   header: {header}")
        print(f"   ticker_col={ticker_col!r} qty_col={qty_col!r} product_col={prod_col!r}")

        for row in rows[1:]:
            raw_ticker = normalize_ticker(_row_value(row, ticker_col)) if ticker_col is not None else ""
            product = _row_value(row, prod_col) if prod_col is not None else ""
            qty = _row_value(row, qty_col) if qty_col is not None else ""
            blob = normalize_ticker(f"{raw_ticker}{product}")
            if wanted and not any(w in blob for w in wanted):
                continue
            print(f"     ticker={raw_ticker or '?'} qty={qty!r} product={str(product)[:40]!r}")
        print()


if __name__ == "__main__":
    main()
