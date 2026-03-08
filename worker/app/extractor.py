from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

from app.config import config
from app.models import Holding, ImportResult, ManualImportResult
from app.parser import normalize_asset_type, parse_b3_xlsx, parse_csv, parse_currency, parse_quantity, normalize_ticker

if TYPE_CHECKING:
    from playwright.sync_api import BrowserContext, Page


class SessionExpiredError(RuntimeError):
    pass


class B3PortfolioExtractor:
    def __init__(self) -> None:
        self.session_file = config.session_file
        self.download_dir = config.download_dir

    def bootstrap_login(self) -> Path:
        from playwright.sync_api import sync_playwright

        self.session_file.parent.mkdir(parents=True, exist_ok=True)
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(
                headless=False,
                args=[
                    "--disable-blink-features=AutomationControlled",
                ],
            )
            context = browser.new_context(
                ignore_https_errors=True,
                locale="pt-BR",
                timezone_id="America/Sao_Paulo",
            )
            page = context.new_page()
            # For manual login, allow Cloudflare/challenge pages to load even if the
            # initial navigation reports an HTTP response error at the network layer.
            self._goto_with_fallback(
                page,
                config.portal_url,
                timeout_ms=config.login_timeout_ms,
                allow_http_error=True,
            )
            page.wait_for_timeout(config.login_timeout_ms)
            context.storage_state(path=str(self.session_file))
            browser.close()
        return self.session_file

    def import_portfolio(self) -> ImportResult:
        from playwright.sync_api import sync_playwright

        if not self.session_file.exists():
            raise SessionExpiredError("B3 login required to refresh session.")

        self.download_dir.mkdir(parents=True, exist_ok=True)
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(
                headless=config.headless,
                args=[
                    "--disable-blink-features=AutomationControlled",
                ],
            )
            context = browser.new_context(
                storage_state=str(self.session_file),
                accept_downloads=True,
                ignore_https_errors=True,
                locale="pt-BR",
                timezone_id="America/Sao_Paulo",
            )
            page = context.new_page()
            holdings = self._load_holdings(context, page)
            browser.close()
        return ImportResult(holdings=holdings)

    def import_manual_file(self, source_file: Path) -> ManualImportResult:
        if not source_file.exists():
            raise RuntimeError(f"Source file not found: {source_file}")
        suffix = source_file.suffix.lower()
        if suffix == ".csv":
            holdings = parse_csv(source_file)
        elif suffix in {".xlsx", ".xlsm"}:
            holdings = parse_b3_xlsx(source_file)
        else:
            raise RuntimeError(f"Unsupported manual import file type: {suffix}")
        return ManualImportResult(holdings=holdings, source="manual_b3_export", source_file=str(source_file))

    def _load_holdings(self, context: "BrowserContext", page: "Page") -> list[Holding]:
        self._open_positions_page(page)
        csv_path = self._download_csv_if_available(page)
        if csv_path is not None:
            return parse_csv(csv_path)
        return self._scrape_table(page)

    def _open_positions_page(self, page: "Page") -> None:
        from playwright.sync_api import TimeoutError

        try:
            self._goto_with_fallback(page, config.portal_url + config.dashboard_path, timeout_ms=config.timeout_ms)
            if self._requires_login(page):
                raise SessionExpiredError("B3 login required to refresh session.")

            self._goto_with_fallback(page, config.portal_url + config.positions_path, timeout_ms=config.timeout_ms)
            if self._requires_login(page):
                raise SessionExpiredError("B3 login required to refresh session.")
        except TimeoutError as exc:
            raise RuntimeError("Timed out while loading the B3 custody page.") from exc

    def _download_csv_if_available(self, page: "Page") -> Path | None:
        from playwright.sync_api import Error, TimeoutError

        selectors = [
            "text=Exportar",
            "text=Export",
            "button:has-text('CSV')",
            "[data-testid='export-csv']",
        ]
        for selector in selectors:
            try:
                page.locator(selector).first.wait_for(state="visible", timeout=3000)
                with page.expect_download(timeout=10000) as download_info:
                    page.locator(selector).first.click()
                download = download_info.value
                path = self.download_dir / "portfolio.csv"
                download.save_as(str(path))
                return path
            except (TimeoutError, Error):
                continue
        return None

    def _scrape_table(self, page: "Page") -> list[Holding]:
        row_selectors = [
            "table tbody tr",
            "[data-testid='custody-table'] tbody tr",
            "[role='table'] [role='row']",
        ]
        rows = None
        for selector in row_selectors:
            locator = page.locator(selector)
            if locator.count() > 0:
                rows = locator
                break

        if rows is None:
            raise RuntimeError("Could not find a holdings table or downloadable CSV in the B3 portal.")

        holdings: list[Holding] = []
        for index in range(rows.count()):
            row = rows.nth(index)
            cells = [text.strip() for text in row.locator("td, [role='cell']").all_inner_texts()]
            if not cells:
                continue
            ticker = normalize_ticker(cells[0] if len(cells) > 0 else None)
            if not ticker:
                continue
            quantity = parse_quantity(cells[1] if len(cells) > 1 else None)
            average_price = parse_currency(cells[2] if len(cells) > 2 else None)
            broker = cells[3] if len(cells) > 3 and cells[3] else None
            holdings.append(
                Holding(
                    ticker=ticker,
                    quantity=quantity,
                    average_price=average_price,
                    broker=broker,
                    asset_type=normalize_asset_type(ticker),
                )
            )
        return holdings

    def _requires_login(self, page: "Page") -> bool:
        body = page.locator("body").inner_text(timeout=5000).lower()
        return "login" in body or "cpf" in body or "autentica" in body

    def _goto_with_fallback(self, page: "Page", url: str, timeout_ms: int, *, allow_http_error: bool = False) -> None:
        from playwright.sync_api import Error

        attempts = [
            ("domcontentloaded", timeout_ms),
            ("load", timeout_ms),
            ("commit", min(timeout_ms, 15000)),
        ]
        errors: list[str] = []
        for wait_until, timeout in attempts:
            try:
                response = page.goto(url, wait_until=wait_until, timeout=timeout)
                if response is None:
                    return
                if response.ok or response.status < 400:
                    return
                if allow_http_error:
                    return
                errors.append(f"{wait_until}: HTTP {response.status}")
            except Error as exc:
                # Chromium sometimes raises net::ERR_HTTP_RESPONSE_CODE_FAILURE for
                # 4xx/5xx navigations even though a response page is available (e.g.
                # Cloudflare challenge). For the manual-login bootstrap, we want to
                # let the page open so the user can complete the challenge.
                if allow_http_error and "ERR_HTTP_RESPONSE_CODE_FAILURE" in str(exc):
                    return
                errors.append(f"{wait_until}: {exc}")
        detail = "; ".join(errors) if errors else "unknown navigation error"
        raise RuntimeError(f"Unable to open B3 portal at {url}. {detail}")
