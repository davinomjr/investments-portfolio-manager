import json
import subprocess
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime
from decimal import Decimal
from pathlib import Path
from shlex import split
from tempfile import NamedTemporaryFile
from typing import Dict, List, Optional

from fastapi import UploadFile
from sqlalchemy import select
from sqlalchemy.orm import Session, joinedload

from app.core.config import settings
from app.models.asset import Asset
from app.models.asset_metadata import AssetMetadata
from app.models.import_job import ImportJob
from app.models.position import Position
from app.schemas.portfolio import AllocationItem, ImportJobResponse, PortfolioResponse, PositionResponse


@dataclass
class HoldingPayload:
    ticker: str
    quantity: float
    average_price: float
    broker: Optional[str]
    asset_type: str
    currency: str = "BRL"
    company_name: Optional[str] = None
    tax_id: Optional[str] = None


def normalize_asset_type(ticker: str, provided_type: Optional[str] = None) -> str:
    if provided_type:
        return provided_type.lower()
    normalized = ticker.upper()
    if normalized.endswith("34"):
        return "bdr"
    if normalized.endswith("11"):
        return "etf_or_fii"
    if normalized[-1:] in {"3", "4", "5", "6"}:
        return "stock"
    return "other"


def trigger_b3_import(db: Session) -> ImportJobResponse:
    job = ImportJob(source="b3", status="running", detail="Import started")
    db.add(job)
    db.commit()
    db.refresh(job)

    try:
        holdings = _run_worker()
        _upsert_positions(db, holdings)
        job.status = "completed"
        job.detail = f"Imported {len(holdings)} positions from B3"
    except RuntimeError as exc:
        job.status = "requires_login"
        job.detail = str(exc)
    except Exception as exc:  # pragma: no cover
        job.status = "failed"
        job.detail = f"Unexpected import error: {exc}"

    job.updated_at = datetime.utcnow()
    db.add(job)
    db.commit()
    db.refresh(job)
    return ImportJobResponse.model_validate(job)


async def import_manual_b3_file(db: Session, upload: UploadFile) -> ImportJobResponse:
    suffix = Path(upload.filename or "").suffix or ".xlsx"
    with NamedTemporaryFile(delete=False, suffix=suffix, dir=settings.upload_dir) as handle:
        temp_path = Path(handle.name)
        handle.write(await upload.read())

    job = ImportJob(source="manual_b3_export", status="running", detail=f"Importing {upload.filename}")
    db.add(job)
    db.commit()
    db.refresh(job)

    try:
        holdings = _run_worker_import_file(temp_path)
        _upsert_positions(db, holdings)
        job.status = "completed"
        job.detail = f"Imported {len(holdings)} positions from {upload.filename}"
    except Exception as exc:
        job.status = "failed"
        job.detail = str(exc)
    finally:
        temp_path.unlink(missing_ok=True)

    job.updated_at = datetime.utcnow()
    db.add(job)
    db.commit()
    db.refresh(job)
    return ImportJobResponse.model_validate(job)


def get_positions_snapshot(db: Session) -> list[PositionResponse]:
    stmt = select(Position).options(joinedload(Position.asset)).order_by(Position.last_updated.desc())
    positions = db.execute(stmt).scalars().all()
    return [
        PositionResponse(
            ticker=position.asset.ticker,
            asset_type=position.asset.asset_type,
            quantity=float(position.quantity),
            avg_price=float(position.avg_price),
            broker=position.broker,
            source=position.source,
            last_updated=position.last_updated,
        )
        for position in positions
    ]


def get_portfolio_snapshot(db: Session) -> PortfolioResponse:
    stmt = select(Position).options(joinedload(Position.asset))
    positions = db.execute(stmt).scalars().all()
    allocations_by_ticker: Dict[str, Dict[str, object]] = defaultdict(dict)
    total_cost = 0.0

    for position in positions:
        market_value = float(Decimal(position.quantity) * Decimal(position.avg_price))
        total_cost += market_value
        allocations_by_ticker[position.asset.ticker] = {
            "asset_type": position.asset.asset_type,
            "market_value": market_value,
        }

    allocations = [
        AllocationItem(
            ticker=ticker,
            asset_type=str(item["asset_type"]),
            market_value=float(item["market_value"]),
            weight=(float(item["market_value"]) / total_cost if total_cost else 0.0),
        )
        for ticker, item in allocations_by_ticker.items()
    ]
    allocations.sort(key=lambda item: item.market_value, reverse=True)

    return PortfolioResponse(
        total_positions=len(positions),
        estimated_cost_basis=total_cost,
        allocations=allocations,
    )


def _run_worker() -> List[HoldingPayload]:
    if settings.worker_command:
        command = split(settings.worker_command)
    else:
        command = [settings.worker_python, "-m", settings.worker_module, "import", "--json"]

    return _run_worker_command(command)


def _run_worker_import_file(source_file: Path) -> List[HoldingPayload]:
    if settings.worker_command:
        command = split(settings.worker_command)
    else:
        command = [settings.worker_python, "-m", settings.worker_module, "import-file", str(source_file), "--json"]

    return _run_worker_command(command)


def _run_worker_command(command: List[str]) -> List[HoldingPayload]:
    result = subprocess.run(
        command,
        cwd=settings.worker_dir,
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        message = result.stderr.strip() or result.stdout.strip() or "Worker execution failed"
        raise RuntimeError(message)

    data = json.loads(result.stdout)
    holdings: List[HoldingPayload] = []
    for item in data.get("holdings", []):
        holdings.append(
            HoldingPayload(
                ticker=item["ticker"].upper(),
                quantity=float(item["quantity"]),
                average_price=float(item["average_price"]),
                broker=item.get("broker"),
                asset_type=normalize_asset_type(item["ticker"], item.get("asset_type")),
                currency=item.get("currency", "BRL"),
                company_name=item.get("company_name"),
                tax_id=item.get("tax_id"),
            )
        )
    return holdings


def _upsert_positions(db: Session, holdings: List[HoldingPayload]) -> None:
    for holding in holdings:
        asset = db.execute(select(Asset).where(Asset.ticker == holding.ticker)).scalar_one_or_none()
        if asset is None:
            asset = Asset(ticker=holding.ticker, asset_type=holding.asset_type, currency=holding.currency)
            db.add(asset)
            db.flush()
        else:
            asset.asset_type = holding.asset_type
            asset.currency = holding.currency

        metadata = db.execute(select(AssetMetadata).where(AssetMetadata.asset_id == asset.id)).scalar_one_or_none()
        if metadata is None:
            metadata = AssetMetadata(
                asset_id=asset.id,
                company_name=holding.company_name,
                tax_id=holding.tax_id,
            )
            db.add(metadata)
        else:
            if holding.company_name:
                metadata.company_name = holding.company_name
            if holding.tax_id:
                metadata.tax_id = holding.tax_id

        position = db.execute(
            select(Position).where(
                Position.user_id == settings.default_user_id,
                Position.asset_id == asset.id,
            )
        ).scalar_one_or_none()
        if position is None:
            position = Position(
                user_id=settings.default_user_id,
                asset_id=asset.id,
                quantity=holding.quantity,
                avg_price=holding.average_price,
                broker=holding.broker,
                source="b3",
            )
            db.add(position)
        else:
            position.quantity = holding.quantity
            position.avg_price = holding.average_price
            position.broker = holding.broker
            position.source = "b3"
            position.last_updated = datetime.utcnow()

    db.commit()
