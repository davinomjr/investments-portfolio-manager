from __future__ import annotations

import csv
from dataclasses import replace
import re
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import Optional
from zipfile import ZipFile

from app.models import Holding


SPREADSHEET_NS = {"a": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
REL_NS = {
    "a": "http://schemas.openxmlformats.org/spreadsheetml/2006/main",
    "r": "http://schemas.openxmlformats.org/officeDocument/2006/relationships",
}
PKG_REL_NS = {"p": "http://schemas.openxmlformats.org/package/2006/relationships"}


def normalize_asset_type(ticker: str) -> str:
    ticker = ticker.upper()
    if ticker.endswith("34"):
        return "bdr"
    if ticker.endswith("11"):
        return "etf_or_fii"
    if ticker[-1:] in {"3", "4", "5", "6"}:
        return "stock"
    return "other"


def parse_currency(value: object) -> float:
    if value is None:
        return 0.0
    if isinstance(value, (int, float)):
        return float(value)
    text = str(value).strip()
    if not text:
        return 0.0
    text = text.replace("R$", "").strip()
    text = _normalize_number_text(text)
    try:
        return float(text)
    except ValueError:
        return 0.0


def parse_quantity(value: object) -> float:
    if value is None:
        return 0.0
    if isinstance(value, (int, float)):
        return float(value)
    text = _normalize_number_text(str(value).strip())
    try:
        return float(text)
    except ValueError:
        return 0.0


def normalize_ticker(value: object) -> str:
    if value is None:
        return ""
    text = re.sub(r"[^A-Za-z0-9]", "", str(value).upper())
    return text


def extract_product_ticker(value: object) -> str:
    if value is None:
        return ""
    text = str(value).strip().upper()
    match = re.match(r"^([A-Z]{4}\d{1,2}[A-Z]?)\b", text)
    if match:
        return match.group(1)
    return normalize_ticker(value)


def parse_csv(path: Path) -> list[Holding]:
    with path.open("r", encoding="utf-8-sig", newline="") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames is None:
            return []
        columns = {column.lower().strip(): column for column in reader.fieldnames}
        ticker_col = _pick(columns, ["ticker", "codigo", "ativo", "asset"])
        quantity_col = _pick(columns, ["quantidade", "quantity", "qtd"])
        avg_price_col = _pick(columns, ["preco medio", "preço médio", "average_price", "pm"])
        broker_col = _pick(columns, ["corretora", "broker", "instituicao"])
        asset_type_col = _pick(columns, ["tipo", "asset_type", "classe"])
        rows = list(reader)

    holdings: list[Holding] = []
    for row in rows:
        ticker = normalize_ticker(row.get(ticker_col) if ticker_col else None)
        if not ticker:
            continue
        asset_type = row.get(asset_type_col) if asset_type_col else None
        holdings.append(
            Holding(
                ticker=ticker,
                quantity=parse_quantity(row.get(quantity_col) if quantity_col else None),
                average_price=parse_currency(row.get(avg_price_col) if avg_price_col else None),
                broker=_optional_text(row.get(broker_col) if broker_col else None),
                asset_type=_optional_text(asset_type).lower() if _optional_text(asset_type) else normalize_asset_type(ticker),
            )
        )
    return holdings


def parse_b3_xlsx(path: Path) -> list[Holding]:
    workbook = _read_xlsx_workbook(path)
    holdings: list[Holding] = []

    for sheet_name, rows in workbook.items():
        normalized_sheet = sheet_name.strip().lower()
        if not rows:
            continue

        header = rows[0]
        data_rows = rows[1:]
        columns = {str(column).lower().strip(): index for index, column in enumerate(header) if str(column).strip()}
        ticker_col = _pick(columns, ["código de negociação", "codigo de negociacao", "ticker", "código", "codigo"])
        quantity_col = _pick(columns, ["quantidade disponível", "quantidade disponivel", "quantidade", "quantity", "qtd"])
        price_col = _pick(columns, ["preço de fechamento", "preco de fechamento", "average_price", "pm"])
        broker_col = _pick(columns, ["instituição", "instituicao", "corretora", "broker"])
        product_col = _pick(columns, ["produto", "ativo", "asset"])
        indexer_col = _pick(columns, ["indexador"])
        maturity_col = _pick(columns, ["vencimento"])
        invested_value_col = _pick(columns, ["valor aplicado"])
        updated_value_col = _pick(columns, ["valor atualizado", "valor bruto", "valor líquido", "valor liquido"])
        type_col = _pick(columns, ["tipo", "classe"])
        tax_id_col = _pick(columns, ["cnpj da empresa", "cnpj do fundo", "cnpj", "cpf/cnpj"])

        if quantity_col is None:
            continue

        for row in data_rows:
            ticker = _derive_ticker(
                row=row,
                sheet_name=normalized_sheet,
                ticker_col=ticker_col,
                product_col=product_col,
                indexer_col=indexer_col,
                maturity_col=maturity_col,
            )
            if not ticker or ticker == "TOTAL":
                continue

            quantity = parse_quantity(_row_value(row, quantity_col))
            if quantity <= 0:
                continue

            average_price = parse_currency(_row_value(row, price_col)) if price_col is not None else 0.0
            if updated_value_col is not None and ("tesouro" in normalized_sheet):
                updated_value = parse_currency(_row_value(row, updated_value_col))
                if updated_value > 0:
                    average_price = (updated_value / quantity) if quantity else 0.0
            if average_price <= 0 and invested_value_col is not None:
                invested_value = parse_currency(_row_value(row, invested_value_col))
                average_price = (invested_value / quantity) if quantity else 0.0

            holdings.append(
                Holding(
                    ticker=ticker,
                    quantity=quantity,
                    average_price=average_price,
                    broker=_optional_text(_row_value(row, broker_col)) if broker_col is not None else None,
                    asset_type=_sheet_asset_type(
                        sheet_name=normalized_sheet,
                        ticker=ticker,
                        security_type=_optional_text(_row_value(row, type_col)) if type_col is not None else None,
                    ),
                    company_name=_derive_company_name(_row_value(row, product_col)),
                    tax_id=_normalize_tax_id(_row_value(row, tax_id_col)) if tax_id_col is not None else None,
                )
            )

    return _merge_holdings(holdings)


def _pick(columns: dict[str, int] | dict[str, str], candidates: list[str]) -> Optional[object]:
    normalized_candidates = {candidate.lower() for candidate in candidates}
    for normalized, original in columns.items():
        if normalized in normalized_candidates:
            return original
    return None


def _sheet_asset_type(sheet_name: str, ticker: str, security_type: Optional[str] = None) -> str:
    if "tesouro" in sheet_name:
        return "government_bond"
    if "empréstimo" in sheet_name or "emprestimo" in sheet_name:
        return "stock"
    if "fundo de investimento" in sheet_name:
        return "fund"
    if sheet_name == "etf":
        return "international_etf" if ticker == "SPYI11" or (security_type and "internacional" in security_type.lower()) else "etf"
    if sheet_name == "acoes":
        if security_type and security_type.upper() == "UNIT":
            return "stock"
        return normalize_asset_type(ticker)
    return normalize_asset_type(ticker)


def _derive_ticker(
    row: list[object],
    sheet_name: str,
    ticker_col: object,
    product_col: object,
    indexer_col: object,
    maturity_col: object,
) -> str:
    ticker = normalize_ticker(_row_value(row, ticker_col))
    if ticker:
        return ticker
    if "tesouro" in sheet_name:
        product = _optional_text(_row_value(row, product_col)) or "TESOURO"
        indexer = _optional_text(_row_value(row, indexer_col)) or "BRASIL"
        maturity = _optional_text(_row_value(row, maturity_col)) or ""
        year = maturity[-4:] if len(maturity) >= 4 else ""
        base = normalize_ticker(product)[:18]
        suffix = normalize_ticker(indexer)[:8]
        return f"TD{suffix}{year or base}"
    return extract_product_ticker(_row_value(row, product_col))


def _merge_holdings(holdings: list[Holding]) -> list[Holding]:
    merged: dict[tuple[str, str], Holding] = {}
    for holding in holdings:
        key = (holding.ticker, holding.asset_type or "other")
        existing = merged.get(key)
        if existing is None:
            merged[key] = replace(holding)
            continue

        total_quantity = existing.quantity + holding.quantity
        if total_quantity > 0:
            existing.average_price = (
                (existing.average_price * existing.quantity) + (holding.average_price * holding.quantity)
            ) / total_quantity
        existing.quantity = total_quantity
        if not existing.broker and holding.broker:
            existing.broker = holding.broker
        if not existing.company_name and holding.company_name:
            existing.company_name = holding.company_name
        if not existing.tax_id and holding.tax_id:
            existing.tax_id = holding.tax_id

    return list(merged.values())


def _optional_text(value: object) -> Optional[str]:
    if value is None:
        return None
    text = str(value).strip()
    if not text or text.lower() == "nan" or text == "-":
        return None
    return text


def _row_value(row: list[object], index: int | str | object) -> object:
    if not isinstance(index, int):
        return None
    if index < 0 or index >= len(row):
        return None
    return row[index]


def _read_xlsx_workbook(path: Path) -> dict[str, list[list[object]]]:
    with ZipFile(path) as archive:
        shared_strings = _read_shared_strings(archive)
        workbook = ET.fromstring(archive.read("xl/workbook.xml"))
        rels = ET.fromstring(archive.read("xl/_rels/workbook.xml.rels"))
        rel_map = {
            rel.get("Id"): rel.get("Target")
            for rel in rels.findall("p:Relationship", PKG_REL_NS)
        }
        sheets: dict[str, list[list[object]]] = {}
        for sheet in workbook.findall("a:sheets/a:sheet", REL_NS):
            name = sheet.get("name", "Sheet")
            rel_id = sheet.get("{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id")
            target = rel_map.get(rel_id)
            if not target:
                continue
            xml_path = "xl/" + target.lstrip("/")
            sheets[name] = _read_sheet_rows(archive, xml_path, shared_strings)
        return sheets


def _read_shared_strings(archive: ZipFile) -> list[str]:
    if "xl/sharedStrings.xml" not in archive.namelist():
        return []
    root = ET.fromstring(archive.read("xl/sharedStrings.xml"))
    values: list[str] = []
    for item in root.findall("a:si", SPREADSHEET_NS):
        text = "".join(node.text or "" for node in item.iterfind(".//a:t", SPREADSHEET_NS))
        values.append(text)
    return values


def _read_sheet_rows(archive: ZipFile, xml_path: str, shared_strings: list[str]) -> list[list[object]]:
    root = ET.fromstring(archive.read(xml_path))
    rows: list[list[object]] = []
    for row in root.findall(".//a:sheetData/a:row", SPREADSHEET_NS):
        parsed_cells: dict[int, object] = {}
        max_index = -1
        for cell in row.findall("a:c", SPREADSHEET_NS):
            ref = cell.get("r", "")
            index = _column_index(ref)
            max_index = max(max_index, index)
            parsed_cells[index] = _cell_value(cell, shared_strings)
        if max_index < 0:
            continue
        rows.append([parsed_cells.get(i, "") for i in range(max_index + 1)])
    return rows


def _cell_value(cell: ET.Element, shared_strings: list[str]) -> object:
    cell_type = cell.get("t")
    if cell_type == "inlineStr":
        return "".join(node.text or "" for node in cell.iterfind(".//a:t", SPREADSHEET_NS))
    value = cell.find("a:v", SPREADSHEET_NS)
    if value is None:
        return ""
    text = value.text or ""
    if cell_type == "s":
        return shared_strings[int(text)] if text.isdigit() and int(text) < len(shared_strings) else ""
    return text


def _column_index(cell_ref: str) -> int:
    letters = "".join(char for char in cell_ref if char.isalpha()).upper()
    index = 0
    for char in letters:
        index = index * 26 + (ord(char) - ord("A") + 1)
    return max(index - 1, 0)


def _normalize_number_text(text: str) -> str:
    text = text.strip()
    if not text:
        return text
    if "," in text and "." in text:
        return text.replace(".", "").replace(",", ".")
    if "," in text:
        return text.replace(".", "").replace(",", ".")
    return text


def _derive_company_name(value: object) -> Optional[str]:
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    parts = text.split(" - ", 1)
    if len(parts) == 2 and extract_product_ticker(parts[0]) == parts[0].strip().upper():
        return _optional_text(parts[1])
    return _optional_text(text)


def _normalize_tax_id(value: object) -> Optional[str]:
    if value is None:
        return None
    digits = re.sub(r"\D", "", str(value))
    return digits or None
