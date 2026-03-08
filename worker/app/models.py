from dataclasses import asdict, dataclass
from typing import Optional


@dataclass
class Holding:
    ticker: str
    quantity: float = 0.0
    average_price: float = 0.0
    broker: Optional[str] = None
    asset_type: Optional[str] = None
    currency: str = "BRL"
    company_name: Optional[str] = None
    tax_id: Optional[str] = None

    def model_dump(self) -> dict:
        return asdict(self)


@dataclass
class ImportResult:
    holdings: list[Holding]
    source: str = "b3"

    def model_dump(self) -> dict:
        return {
            "holdings": [holding.model_dump() for holding in self.holdings],
            "source": self.source,
        }


@dataclass
class ManualImportResult(ImportResult):
    source_file: str = ""

    def model_dump(self) -> dict:
        payload = super().model_dump()
        payload["source_file"] = self.source_file
        return payload
