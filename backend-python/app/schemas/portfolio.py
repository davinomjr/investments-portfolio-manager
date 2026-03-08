from datetime import datetime
from typing import Optional

from pydantic import BaseModel


class ImportJobResponse(BaseModel):
    id: int
    source: str
    status: str
    detail: Optional[str] = None
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True


class PositionResponse(BaseModel):
    ticker: str
    asset_type: str
    quantity: float
    avg_price: float
    broker: Optional[str] = None
    source: str
    last_updated: datetime


class AllocationItem(BaseModel):
    ticker: str
    asset_type: str
    market_value: float
    weight: float


class PortfolioResponse(BaseModel):
    total_positions: int
    estimated_cost_basis: float
    allocations: list[AllocationItem]


class ImportFileResponse(BaseModel):
    filename: str
    imported_positions: int


class QuarterlyResultItem(BaseModel):
    ticker: str
    company_name: Optional[str] = None
    asset_type: str
    report_date: Optional[str] = None
    revenue: Optional[float] = None
    net_income: Optional[float] = None
    ebitda: Optional[float] = None
    net_margin: Optional[float] = None
    highlights: list[str]
    status: str
    message: Optional[str] = None


class QuarterlyResultsResponse(BaseModel):
    provider: str
    configured: bool
    message: Optional[str] = None
    items: list[QuarterlyResultItem]
