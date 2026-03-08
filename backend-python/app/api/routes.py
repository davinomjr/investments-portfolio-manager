from fastapi import APIRouter, Depends, File, HTTPException, UploadFile, status
from sqlalchemy.orm import Session

from app.db.session import get_db
from app.schemas.portfolio import ImportJobResponse, PortfolioResponse, PositionResponse
from app.services.portfolio import (
    get_portfolio_snapshot,
    get_positions_snapshot,
    import_manual_b3_file,
    trigger_b3_import,
)
from app.services.quarterly_results import get_latest_quarterly_results
from app.schemas.portfolio import QuarterlyResultsResponse


router = APIRouter()


@router.post("/portfolio/import-b3", response_model=ImportJobResponse, status_code=status.HTTP_202_ACCEPTED)
def import_b3_portfolio(db: Session = Depends(get_db)) -> ImportJobResponse:
    try:
        return trigger_b3_import(db)
    except RuntimeError as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc


@router.post("/portfolio/import-file", response_model=ImportJobResponse, status_code=status.HTTP_202_ACCEPTED)
async def import_manual_file(
    file: UploadFile = File(...),
    db: Session = Depends(get_db),
) -> ImportJobResponse:
    filename = file.filename or ""
    if not filename.lower().endswith((".xlsx", ".xlsm", ".csv")):
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="Only .xlsx, .xlsm, and .csv files are supported.")

    try:
        return await import_manual_b3_file(db, file)
    except RuntimeError as exc:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(exc)) from exc


@router.get("/portfolio", response_model=PortfolioResponse)
def get_portfolio(db: Session = Depends(get_db)) -> PortfolioResponse:
    return get_portfolio_snapshot(db)


@router.get("/positions", response_model=list[PositionResponse])
def get_positions(db: Session = Depends(get_db)) -> list[PositionResponse]:
    return get_positions_snapshot(db)


@router.get("/stocks/latest-results", response_model=QuarterlyResultsResponse)
def get_latest_results(db: Session = Depends(get_db)) -> QuarterlyResultsResponse:
    return get_latest_quarterly_results(db)
