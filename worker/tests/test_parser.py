"""Tests for the B3 spreadsheet parser.

Run with ``pytest`` (preferred) or directly: ``python -m tests.test_parser``
from the ``worker/`` directory. No third-party deps required — the fixture
workbook is built from raw XML so we don't depend on openpyxl.
"""

from __future__ import annotations

import io
from pathlib import Path
from zipfile import ZipFile

from app.models import Holding
from app.parser import _is_holdings_sheet, _merge_holdings, parse_b3_xlsx


def _sheet_xml(rows: list[list[object]]) -> str:
    """Build a worksheet XML body from rows of (header/data) cell values."""
    ns = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
    out = [f'<worksheet xmlns="{ns}"><sheetData>']
    for r, cells in enumerate(rows, start=1):
        out.append(f'<row r="{r}">')
        for c, value in enumerate(cells):
            ref = f"{chr(ord('A') + c)}{r}"
            if isinstance(value, (int, float)):
                out.append(f'<c r="{ref}"><v>{value}</v></c>')
            else:
                out.append(f'<c r="{ref}" t="inlineStr"><is><t>{value}</t></is></c>')
        out.append("</row>")
    out.append("</sheetData></worksheet>")
    return "".join(out)


def _build_xlsx(sheets: dict[str, list[list[object]]]) -> bytes:
    """Assemble a minimal .xlsx the parser can read."""
    ns_main = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
    ns_rel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    ns_pkg = "http://schemas.openxmlformats.org/package/2006/relationships"

    sheet_entries = []
    rel_entries = []
    for i, name in enumerate(sheets, start=1):
        sheet_entries.append(
            f'<sheet name="{name}" sheetId="{i}" r:id="rId{i}"/>'
        )
        rel_entries.append(
            f'<Relationship Id="rId{i}" '
            f'Type="{ns_rel}/worksheet" Target="worksheets/sheet{i}.xml"/>'
        )

    workbook_xml = (
        f'<workbook xmlns="{ns_main}" xmlns:r="{ns_rel}">'
        f'<sheets>{"".join(sheet_entries)}</sheets></workbook>'
    )
    rels_xml = (
        f'<Relationships xmlns="{ns_pkg}">{"".join(rel_entries)}</Relationships>'
    )

    buffer = io.BytesIO()
    with ZipFile(buffer, "w") as archive:
        archive.writestr("xl/workbook.xml", workbook_xml)
        archive.writestr("xl/_rels/workbook.xml.rels", rels_xml)
        for i, rows in enumerate(sheets.values(), start=1):
            archive.writestr(f"xl/worksheets/sheet{i}.xml", _sheet_xml(rows))
    return buffer.getvalue()


def test_is_holdings_sheet():
    assert _is_holdings_sheet("Acoes")
    assert _is_holdings_sheet("BDR")
    assert _is_holdings_sheet("Tesouro Direto")
    # Informational / lending tabs must be skipped.
    assert not _is_holdings_sheet("Empréstimo de Ativos")
    assert not _is_holdings_sheet("Emprestimo de Ativos")
    assert not _is_holdings_sheet("Proventos Recebidos")
    assert not _is_holdings_sheet("Eventos Provisionados")


def test_lending_sheet_does_not_double_count(tmp_path: Path):
    """A ticker listed in both Acoes and Empréstimo must not be summed."""
    header = ["Código de Negociação", "Quantidade", "Preço de Fechamento"]
    sheets = {
        "Acoes": [header, ["BBAS3", 1300, "10.00"]],
        # B3 repeats the lent shares here — must be ignored.
        "Empréstimo de Ativos": [header, ["BBAS3", 1300, "10.00"]],
    }
    path = tmp_path / "posicao.xlsx"
    path.write_bytes(_build_xlsx(sheets))

    holdings = parse_b3_xlsx(path)
    by_ticker = {h.ticker: h for h in holdings}

    assert "BBAS3" in by_ticker
    assert by_ticker["BBAS3"].quantity == 1300


def test_multiple_brokers_in_acoes_still_sum(tmp_path: Path):
    """Genuine duplicate rows within the custody sheet should still aggregate."""
    header = ["Código de Negociação", "Quantidade", "Preço de Fechamento", "Instituição"]
    sheets = {
        "Acoes": [
            header,
            ["PETR4", 100, "30.00", "Corretora A"],
            ["PETR4", 50, "32.00", "Corretora B"],
        ],
    }
    path = tmp_path / "posicao.xlsx"
    path.write_bytes(_build_xlsx(sheets))

    holdings = parse_b3_xlsx(path)
    by_ticker = {h.ticker: h for h in holdings}

    assert by_ticker["PETR4"].quantity == 150


def test_merge_holdings_sums_same_key():
    merged = _merge_holdings(
        [
            Holding(ticker="ITSA4", quantity=100, average_price=10.0, asset_type="stock"),
            Holding(ticker="ITSA4", quantity=100, average_price=12.0, asset_type="stock"),
        ]
    )
    assert len(merged) == 1
    assert merged[0].quantity == 200
    assert merged[0].average_price == 11.0


if __name__ == "__main__":
    import tempfile

    test_is_holdings_sheet()
    test_merge_holdings_sums_same_key()
    with tempfile.TemporaryDirectory() as d:
        test_lending_sheet_does_not_double_count(Path(d))
        test_multiple_brokers_in_acoes_still_sum(Path(d))
    print("ok")
