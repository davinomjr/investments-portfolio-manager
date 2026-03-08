from __future__ import annotations

import csv
import io
import re
import unicodedata
from dataclasses import dataclass
from datetime import date, datetime
from pathlib import Path
from zipfile import ZipFile

import httpx
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.core.config import settings
from app.models.asset import Asset
from app.models.asset_metadata import AssetMetadata
from app.models.position import Position
from app.schemas.portfolio import QuarterlyResultItem, QuarterlyResultsResponse


SUPPORTED_ASSET_TYPES = {"stock", "bdr"}
REVENUE_CODES = {"3.01"}
NET_INCOME_CODES = {"3.11", "3.13", "3.11.01"}
SCALE_MAP = {
    "UNIDADE": 1.0,
    "UNIDADES": 1.0,
    "MIL": 1000.0,
    "MILHAR": 1000.0,
    "MILHARES": 1000.0,
    "MILHÃO": 1_000_000.0,
    "MILHOES": 1_000_000.0,
    "MILHÕES": 1_000_000.0,
    "R$ MIL": 1000.0,
    "R$ MILHÕES": 1_000_000.0,
}


@dataclass
class TrackedAsset:
    ticker: str
    asset_type: str
    company_name: str | None
    tax_id: str | None


def get_latest_quarterly_results(db: Session) -> QuarterlyResultsResponse:
    tracked = _load_tracked_assets(db)
    if not tracked:
        return QuarterlyResultsResponse(
            provider="cvm_itr",
            configured=True,
            message="No stock positions were found in the imported portfolio.",
            items=[],
        )

    if not any(asset.company_name or asset.tax_id for asset in tracked):
        return QuarterlyResultsResponse(
            provider="cvm_itr",
            configured=True,
            message="Re-upload the B3 workbook once so issuer metadata is stored before CVM matching runs.",
            items=[
                QuarterlyResultItem(
                    ticker=asset.ticker,
                    company_name=None,
                    asset_type=asset.asset_type,
                    highlights=[],
                    status="metadata_missing",
                    message="Issuer metadata is missing for this position.",
                )
                for asset in tracked
            ],
        )

    rows, report_year = _load_latest_itr_rows()
    if not rows:
        return QuarterlyResultsResponse(
            provider="cvm_itr",
            configured=True,
            message="CVM quarterly files could not be loaded right now.",
            items=[
                QuarterlyResultItem(
                    ticker=asset.ticker,
                    company_name=asset.company_name,
                    asset_type=asset.asset_type,
                    highlights=[],
                    status="unavailable",
                    message="CVM ITR dataset unavailable.",
                )
                for asset in tracked
            ],
        )

    tax_index = _index_by_tax_id(rows)
    name_index = _index_by_company(rows)
    items = [_build_result(asset, tax_index, name_index) for asset in tracked]

    return QuarterlyResultsResponse(
        provider="cvm_itr",
        configured=True,
        message=f"Source: CVM ITR {report_year}. Latest reported quarter is inferred from filing periods.",
        items=items,
    )


def _load_tracked_assets(db: Session) -> list[TrackedAsset]:
    stmt = (
        select(Asset.ticker, Asset.asset_type, AssetMetadata.company_name, AssetMetadata.tax_id)
        .join(Position, Position.asset_id == Asset.id)
        .outerjoin(AssetMetadata, AssetMetadata.asset_id == Asset.id)
        .where(Position.source.in_(["b3"]), Asset.asset_type.in_(SUPPORTED_ASSET_TYPES))
        .distinct()
    )
    rows = db.execute(stmt).all()
    return [
        TrackedAsset(
            ticker=row.ticker,
            asset_type=row.asset_type,
            company_name=row.company_name,
            tax_id=row.tax_id,
        )
        for row in rows
    ]


def _build_result(
    asset: TrackedAsset,
    tax_index: dict[str, list[dict[str, str]]],
    name_index: dict[str, list[dict[str, str]]],
) -> QuarterlyResultItem:
    company_rows = []
    if asset.tax_id:
        company_rows = tax_index.get(asset.tax_id, [])
    if not company_rows and asset.company_name:
        company_rows = _match_company_rows(asset.company_name, name_index)

    if not company_rows:
        return QuarterlyResultItem(
            ticker=asset.ticker,
            company_name=asset.company_name,
            asset_type=asset.asset_type,
            highlights=[],
            status="unavailable",
            message="No matching company was found in CVM ITR data for this holding.",
        )

    quarter_rows = _select_latest_quarter_rows(company_rows)
    if not quarter_rows:
        return QuarterlyResultItem(
            ticker=asset.ticker,
            company_name=asset.company_name or company_rows[0].get("DENOM_CIA"),
            asset_type=asset.asset_type,
            highlights=[],
            status="unavailable",
            message="No quarter-length DRE rows were found for the latest filing period.",
        )

    revenue = _extract_revenue_metric(quarter_rows)
    net_income = _extract_metric(quarter_rows, NET_INCOME_CODES, ["lucro", "preju", "periodo"])
    report_date = quarter_rows[0].get("DT_FIM_EXERC") or quarter_rows[0].get("DT_REFER")
    net_margin = (net_income / revenue * 100.0) if revenue not in (None, 0) and net_income is not None else None

    if revenue is None and net_income is None:
        return QuarterlyResultItem(
            ticker=asset.ticker,
            company_name=asset.company_name or quarter_rows[0].get("DENOM_CIA"),
            asset_type=asset.asset_type,
            report_date=report_date,
            highlights=[],
            status="unavailable",
            message="Matched CVM company, but revenue and net income were not found in the latest DRE quarter rows.",
        )

    return QuarterlyResultItem(
        ticker=asset.ticker,
        company_name=asset.company_name or quarter_rows[0].get("DENOM_CIA"),
        asset_type=asset.asset_type,
        report_date=report_date,
        revenue=revenue,
        net_income=net_income,
        ebitda=None,
        net_margin=net_margin,
        highlights=_build_highlights(revenue, net_income, net_margin),
        status="ok",
        message=None,
    )


def _load_latest_itr_rows() -> tuple[list[dict[str, str]], int | None]:
    current_year = datetime.now().year
    candidates = [current_year, current_year - 1, current_year - 2]
    last_error: Exception | None = None

    for year in candidates:
        try:
            zip_path = _ensure_itr_zip(year)
            rows = _read_dre_rows(zip_path)
            if rows:
                return rows, year
        except Exception as exc:  # pragma: no cover
            last_error = exc
            continue

    if last_error:
        return [], None
    return [], None


def _ensure_itr_zip(year: int) -> Path:
    settings.data_cache_dir.mkdir(parents=True, exist_ok=True)
    target = settings.data_cache_dir / f"itr_cia_aberta_{year}.zip"
    if target.exists():
        return target

    url = f"{settings.cvm_itr_base_url}/itr_cia_aberta_{year}.zip"
    response = httpx.get(url, timeout=60.0)
    response.raise_for_status()
    target.write_bytes(response.content)
    return target


def _read_dre_rows(zip_path: Path) -> list[dict[str, str]]:
    with ZipFile(zip_path) as archive:
        names = archive.namelist()
        candidates = [name for name in names if "DRE_con" in name] or [name for name in names if "DRE_ind" in name]
        if not candidates:
            return []
        rows: list[dict[str, str]] = []
        for name in candidates:
            content = archive.read(name)
            rows.extend(_parse_cvm_csv(content))
        return rows


def _parse_cvm_csv(content: bytes) -> list[dict[str, str]]:
    text = content.decode("latin-1")
    reader = csv.DictReader(io.StringIO(text), delimiter=";")
    return [row for row in reader]


def _index_by_tax_id(rows: list[dict[str, str]]) -> dict[str, list[dict[str, str]]]:
    indexed: dict[str, list[dict[str, str]]] = {}
    for row in rows:
        tax_id = _normalize_tax_id(row.get("CNPJ_CIA"))
        if not tax_id:
            continue
        indexed.setdefault(tax_id, []).append(row)
    return indexed


def _index_by_company(rows: list[dict[str, str]]) -> dict[str, list[dict[str, str]]]:
    indexed: dict[str, list[dict[str, str]]] = {}
    for row in rows:
        normalized = _normalize_company_name(row.get("DENOM_CIA"))
        if not normalized:
            continue
        indexed.setdefault(normalized, []).append(row)
    return indexed


def _match_company_rows(company_name: str, name_index: dict[str, list[dict[str, str]]]) -> list[dict[str, str]]:
    normalized = _normalize_company_name(company_name)
    if normalized in name_index:
        return name_index[normalized]

    target_tokens = set(normalized.split())
    best_key = None
    best_score = 0.0
    for candidate_key in name_index.keys():
        candidate_tokens = set(candidate_key.split())
        if not candidate_tokens or not target_tokens:
            continue
        score = len(target_tokens & candidate_tokens) / len(target_tokens | candidate_tokens)
        if score > best_score:
            best_key = candidate_key
            best_score = score
    if best_key and best_score >= 0.45:
        return name_index[best_key]
    return []


def _select_latest_quarter_rows(rows: list[dict[str, str]]) -> list[dict[str, str]]:
    latest_end = max((_parse_date(row.get("DT_FIM_EXERC") or row.get("DT_REFER")) for row in rows), default=None)
    if not latest_end:
        return []

    current_rows = [row for row in rows if _parse_date(row.get("DT_FIM_EXERC") or row.get("DT_REFER")) == latest_end]
    current_rows = [row for row in current_rows if (row.get("ORDEM_EXERC") or "").upper() == "ÚLTIMO"]
    if not current_rows:
        current_rows = [row for row in rows if _parse_date(row.get("DT_FIM_EXERC") or row.get("DT_REFER")) == latest_end]

    periods: dict[tuple[date | None, date | None], list[dict[str, str]]] = {}
    for row in current_rows:
        start = _parse_date(row.get("DT_INI_EXERC"))
        end = _parse_date(row.get("DT_FIM_EXERC") or row.get("DT_REFER"))
        periods.setdefault((start, end), []).append(row)

    quarter_groups = []
    for period, period_rows in periods.items():
        start, end = period
        if not start or not end:
            continue
        days = (end - start).days
        if 70 <= days <= 120:
            quarter_groups.append((days, period_rows))
    if quarter_groups:
        quarter_groups.sort(key=lambda item: item[0])
        return _latest_version_rows(quarter_groups[0][1])

    return _latest_version_rows(current_rows)


def _latest_version_rows(rows: list[dict[str, str]]) -> list[dict[str, str]]:
    best_version = max((_parse_int(row.get("VERSAO")) for row in rows), default=0)
    return [row for row in rows if _parse_int(row.get("VERSAO")) == best_version]


def _extract_metric(rows: list[dict[str, str]], codes: set[str], description_tokens: list[str]) -> float | None:
    exact_code_rows = [row for row in rows if (row.get("CD_CONTA") or "") in codes]
    if exact_code_rows:
        return _row_value(exact_code_rows[0])

    for row in rows:
        description = _normalize_company_name(row.get("DS_CONTA"))
        if all(token in description for token in description_tokens):
            return _row_value(row)
    return None


def _extract_revenue_metric(rows: list[dict[str, str]]) -> float | None:
    standard_revenue = _extract_metric(rows, REVENUE_CODES, ["receita"])
    if standard_revenue not in (None, 0):
        return standard_revenue

    candidates: list[tuple[float, dict[str, str]]] = []
    for row in rows:
        description = _normalize_company_name(row.get("DS_CONTA"))
        if not description or "RECEITA" not in description:
            continue
        if "FINANCEIRA" in description:
            continue
        value = _row_value(row)
        if value is None or value <= 0:
            continue
        candidates.append((value, row))

    if not candidates:
        return standard_revenue

    # Prefer explicit service/operating revenue lines when the standard top-line is zero.
    prioritized_tokens = [
        ("PRESTACAO", "SERVICOS"),
        ("OUTRAS", "RECEITAS", "OPERACIONAIS"),
        ("RECEITAS", "OPERACIONAIS"),
    ]
    for tokens in prioritized_tokens:
        matched = [
            value
            for value, row in candidates
            if all(token in _normalize_company_name(row.get("DS_CONTA")) for token in tokens)
        ]
        if matched:
            return max(matched)

    return max(value for value, _row in candidates)


def _row_value(row: dict[str, str]) -> float | None:
    raw = row.get("VL_CONTA")
    if raw is None or raw == "":
        return None
    try:
        value = _parse_cvm_number(str(raw))
    except ValueError:
        return None
    scale = SCALE_MAP.get((row.get("ESCALA_MOEDA") or "").strip().upper(), 1.0)
    return value * scale


def _parse_date(value: str | None) -> date | None:
    if not value:
        return None
    for fmt in ("%Y-%m-%d", "%d/%m/%Y"):
        try:
            return datetime.strptime(value[:10], fmt).date()
        except ValueError:
            continue
    return None


def _parse_int(value: str | None) -> int:
    try:
        return int(value or "0")
    except ValueError:
        return 0


def _normalize_tax_id(value: str | None) -> str | None:
    if not value:
        return None
    digits = re.sub(r"\D", "", value)
    return digits or None


def _normalize_company_name(value: str | None) -> str:
    if not value:
        return ""
    text = unicodedata.normalize("NFKD", value)
    text = "".join(char for char in text if not unicodedata.combining(char))
    text = text.upper()
    replacements = {
        " S.A.": " SA",
        " S/A": " SA",
        " BCO ": " BANCO ",
        " CIA ": " COMPANHIA ",
        " ENERGIA BRASIL ": " ENERGIA BRASIL ",
        " PARTICIPACOES ": " PARTICIPACOES ",
    }
    for source, target in replacements.items():
        text = text.replace(source, target)
    text = re.sub(r"[^A-Z0-9 ]", " ", text)
    text = re.sub(r"\s+", " ", text).strip()
    return text


def _parse_cvm_number(raw: str) -> float:
    text = raw.strip()
    if not text:
        raise ValueError("empty numeric value")
    if "," in text and "." in text:
        text = text.replace(".", "").replace(",", ".")
    elif "," in text:
        text = text.replace(".", "").replace(",", ".")
    return float(text)


def _build_highlights(revenue: float | None, net_income: float | None, net_margin: float | None) -> list[str]:
    highlights = []
    if revenue is not None:
        highlights.append(f"Revenue {format_brl(revenue)}")
    if net_income is not None:
        highlights.append(f"Net income {format_brl(net_income)}")
    if net_margin is not None:
        highlights.append(f"Net margin {net_margin:.1f}%")
    return highlights


def format_brl(value: float) -> str:
    sign = "-" if value < 0 else ""
    absolute = abs(value)
    if absolute >= 1_000_000_000:
        return f"{sign}R$ {absolute / 1_000_000_000:.2f}B"
    if absolute >= 1_000_000:
        return f"{sign}R$ {absolute / 1_000_000:.2f}M"
    if absolute >= 1_000:
        return f"{sign}R$ {absolute / 1_000:.1f}K"
    return f"{sign}R$ {absolute:.2f}"
