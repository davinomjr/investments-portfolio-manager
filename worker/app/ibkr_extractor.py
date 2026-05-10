"""IBKR Flex Web Service extractor."""
from __future__ import annotations

import os
import time
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET

from app.models import Holding, ImportResult

# Use the canonical v3 endpoints. The legacy /Universal/servlet/ paths still
# exist but are degraded — they intermittently return 1001 even when the
# query is healthy, and rate-limit aggressively. The /AccountManagement/
# FlexWebService/ paths are the v3-documented endpoints and behave correctly.
INITIATE_URL = (
    "https://ndcdyn.interactivebrokers.com/AccountManagement/FlexWebService/SendRequest"
    "?t={token}&q={query_id}&v=3"
)
FETCH_URL = (
    "https://gdcdyn.interactivebrokers.com/AccountManagement/FlexWebService/GetStatement"
    "?t={token}&q={ref_code}&v=3"
)

ASSET_CLASS_MAP = {
    "STK": "international_stock",
    "FND": "international_etf",
    "ETF": "international_etf",
    "OPT": "international_option",
    "FUT": "international_future",
    "BOND": "international_bond",
    "CASH": "international_cash",
    "WAR": "international_warrant",
}

MAX_RETRIES = 5
POLL_INTERVAL = 10  # seconds


def _fetch(url: str, timeout: int = 60) -> str:
    with urllib.request.urlopen(url, timeout=timeout) as resp:
        return resp.read().decode("utf-8")


class IbkrExtractor:
    def __init__(self) -> None:
        self.token = os.environ.get("IBKR_FLEX_TOKEN", "")
        self.query_id = os.environ.get("IBKR_FLEX_QUERY_ID", "")
        if not self.token or not self.query_id:
            raise RuntimeError("IBKR_FLEX_TOKEN and IBKR_FLEX_QUERY_ID must be set")

    def _initiate(self) -> tuple[str, str]:
        # IBKR sometimes returns "Statement could not be generated at this
        # time. Please try again shortly." — retry a few times before failing
        # the whole sync.
        url = INITIATE_URL.format(token=self.token, query_id=self.query_id)
        last_error = ""
        for attempt in range(1, MAX_RETRIES + 1):
            try:
                raw = _fetch(url)
            except (urllib.error.URLError, OSError) as exc:
                last_error = str(exc)
                if attempt == MAX_RETRIES:
                    raise RuntimeError(f"IBKR Flex initiate failed after {MAX_RETRIES} attempts: {exc}") from exc
                time.sleep(POLL_INTERVAL)
                continue
            root = ET.fromstring(raw)
            status = root.findtext("Status")
            if status == "Success":
                ref_code = root.findtext("ReferenceCode") or ""
                base_url = root.findtext("Url") or ""
                return ref_code, base_url
            last_error = root.findtext("ErrorMessage") or raw
            if attempt < MAX_RETRIES:
                time.sleep(POLL_INTERVAL)
        raise RuntimeError(f"IBKR Flex initiate failed after {MAX_RETRIES} attempts: {last_error}")

    def _fetch_report(self, ref_code: str) -> str:
        url = FETCH_URL.format(token=self.token, ref_code=ref_code)
        for attempt in range(1, MAX_RETRIES + 1):
            try:
                raw = _fetch(url)
            except (urllib.error.URLError, OSError) as exc:
                if attempt == MAX_RETRIES:
                    raise RuntimeError(f"IBKR Flex fetch failed after {MAX_RETRIES} attempts: {exc}") from exc
                time.sleep(POLL_INTERVAL)
                continue
            if "<FlexQueryResponse" in raw:
                return raw
            try:
                root = ET.fromstring(raw)
                status = root.findtext("Status")
                if status == "Success":
                    return raw
                error = root.findtext("ErrorMessage") or ""
                if "not ready" not in error.lower() and attempt == MAX_RETRIES:
                    raise RuntimeError(f"IBKR Flex fetch failed: {error}")
            except ET.ParseError:
                pass
            if attempt < MAX_RETRIES:
                time.sleep(POLL_INTERVAL)
        raise RuntimeError("IBKR Flex report not ready after max retries")

    def _parse(self, xml_str: str) -> list[Holding]:
        root = ET.fromstring(xml_str)
        holdings: list[Holding] = []
        for pos in root.iter("OpenPosition"):
            symbol = pos.get("symbol", "").strip()
            if not symbol:
                continue
            asset_class_raw = pos.get("assetClass", "STK")
            sub_category = pos.get("subCategory", "").upper()
            # IBKR classifies ETFs under assetClass=STK; subCategory=ETF identifies them
            if sub_category == "ETF":
                asset_type = "international_etf"
            else:
                asset_type = ASSET_CLASS_MAP.get(asset_class_raw, f"international_{asset_class_raw.lower()}" if asset_class_raw else "international_stock")
            quantity = float(pos.get("position") or pos.get("quantity") or 0)
            cost_basis = float(pos.get("costBasisMoney") or 0)
            avg_price = (cost_basis / quantity) if quantity else 0.0
            currency = pos.get("currency", "USD")
            description = pos.get("description", "")
            holdings.append(Holding(
                ticker=symbol,
                quantity=quantity,
                average_price=avg_price,
                broker="ibkr",
                asset_type=asset_type,
                currency=currency,
                company_name=description or None,
            ))
        return holdings

    def import_portfolio(self) -> ImportResult:
        ref_code, _base_url = self._initiate()
        xml_str = self._fetch_report(ref_code)
        holdings = self._parse(xml_str)
        return ImportResult(holdings=holdings, source="ibkr")
