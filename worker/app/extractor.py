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


_STEALTH_ARGS = [
    "--disable-blink-features=AutomationControlled",
    "--no-sandbox",
    "--disable-dev-shm-usage",
    "--disable-gpu",
]

_STEALTH_UA = (
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)


class B3PortfolioExtractor:
    def __init__(self) -> None:
        self.session_file = config.session_file
        self.download_dir = config.download_dir

    def _new_context(self, playwright, *, headless: bool, accept_downloads: bool = False):
        """Launch browser and create a context with stealth settings."""
        browser = playwright.chromium.launch(headless=headless, args=_STEALTH_ARGS)
        storage = str(self.session_file) if self.session_file.exists() else None
        context = browser.new_context(
            storage_state=storage,
            user_agent=_STEALTH_UA,
            viewport={"width": 1920, "height": 1080},
            accept_downloads=accept_downloads,
            ignore_https_errors=True,
            locale="pt-BR",
            timezone_id="America/Sao_Paulo",
        )
        page = context.new_page()
        page.add_init_script("Object.defineProperty(navigator, 'webdriver', {get: () => undefined})")
        return browser, context, page

    def bootstrap_login(self) -> Path:
        from playwright.sync_api import sync_playwright

        self.session_file.parent.mkdir(parents=True, exist_ok=True)
        with sync_playwright() as playwright:
            browser, context, page = self._new_context(playwright, headless=False)
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

        self.download_dir.mkdir(parents=True, exist_ok=True)
        with sync_playwright() as playwright:
            browser, context, page = self._new_context(
                playwright, headless=config.headless, accept_downloads=True,
            )

            # Navigate to dashboard; B3 is a React SPA that returns 404 at the
            # network layer for all routes, so we allow HTTP errors here.
            self._goto_with_fallback(
                page,
                config.portal_url + config.dashboard_path,
                timeout_ms=config.timeout_ms,
                allow_http_error=True,
            )
            # Wait for the React app to render and any client-side redirect
            # (e.g. to investidor.b3.com.br/login when session is expired).
            try:
                page.wait_for_load_state("networkidle", timeout=15000)
            except Exception:
                pass
            # Extra pause for the SPA auth check to complete
            page.wait_for_timeout(2000)

            if self._requires_login(page):
                if config.b3_cpf and config.b3_password:
                    self._auto_login(page, context)
                    # Wait for the SPA auth state to fully settle before navigating
                    page.wait_for_timeout(6000)
                else:
                    raise SessionExpiredError("B3 login required to refresh session.")

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
        file_path = self._download_file_if_available(page)
        if file_path is not None:
            suffix = file_path.suffix.lower()
            if suffix in {".xlsx", ".xlsm"}:
                return parse_b3_xlsx(file_path)
            return parse_csv(file_path)
        return self._scrape_table(page)

    def _auto_login(self, page: "Page", context: "BrowserContext") -> None:
        """Fill CPF and password on the B3 login page automatically."""
        from playwright.sync_api import TimeoutError

        try:
            # Step 1: fill CPF
            page.wait_for_selector("input:visible", timeout=10000)
            cpf_input = page.locator("input:visible").first
            cpf_input.click()
            cpf_input.press_sequentially(config.b3_cpf, delay=60)

            # Step 2: submit via Enter (focus is on the CPF input)
            page.wait_for_timeout(1000)
            page.keyboard.press("Enter")
        except TimeoutError as exc:
            self._dump_debug_context(page, reason="auto-login-no-cpf-field")
            raise SessionExpiredError("Auto-login failed — CPF field not found.") from exc

        try:
            # There may be an intermediate Azure B2C "Continuar" page between
            # CPF and password — handle it by pressing Enter to advance.
            page.wait_for_timeout(1500)
            if not page.locator("input[type='password']:visible").count():
                page.keyboard.press("Enter")

            # Wait for the password field to appear
            page.wait_for_selector("input[type='password']:visible", timeout=15000)
            pwd = page.locator("input[type='password']:visible").first
            pwd.click()
            pwd.press_sequentially(config.b3_password, delay=60)

            # Submit via Enter (focus is on the password field)
            page.wait_for_timeout(1000)
            page.keyboard.press("Enter")
        except TimeoutError as exc:
            self._dump_debug_context(page, reason="auto-login-no-password-field")
            raise SessionExpiredError("Auto-login failed — password field not found.") from exc

        # B3 sometimes throws an email 2FA challenge after CPF+password. Pause
        # for an out-of-band code drop at /tmp/b3-2fa-code so the worker can be
        # driven from a remote session where stdin isn't available.
        try:
            page.wait_for_function(
                "() => document.title.includes('Código de autenticação') "
                "|| (window.location.hostname.includes('investidor.b3.com.br') "
                "&& !window.location.href.includes('/login') "
                "&& !window.location.hostname.includes('b2clogin') "
                "&& document.body && document.body.innerText.trim().length > 100)",
                timeout=60000,
            )
            if "Código de autenticação" in (page.title() or ""):
                from pathlib import Path as _P
                import sys as _sys
                import time as _t
                code_file = _P("/tmp/b3-2fa-code")
                if code_file.exists():
                    code_file.unlink()
                print("[b3-2fa] waiting for /tmp/b3-2fa-code (max 5 min)…", file=_sys.stderr, flush=True)
                deadline = _t.time() + 300
                while _t.time() < deadline and not code_file.exists():
                    _t.sleep(2)
                if not code_file.exists():
                    raise SessionExpiredError("2FA code was not provided in time.")
                code = code_file.read_text().strip()
                code_file.unlink()
                print(f"[b3-2fa] entering code ({len(code)} digits)…", file=_sys.stderr, flush=True)
                code_input = page.locator("input:visible").first
                code_input.click()
                code_input.press_sequentially(code, delay=80)
                page.wait_for_timeout(500)
                page.keyboard.press("Enter")

                page.wait_for_function(
                    "() => window.location.hostname.includes('investidor.b3.com.br') "
                    "&& !window.location.href.includes('/login') "
                    "&& !window.location.hostname.includes('b2clogin') "
                    "&& document.body && document.body.innerText.trim().length > 100",
                    timeout=60000,
                )
            # If the "Já baixou o App B3?" popup is present, dismiss it
            try:
                page.locator("text=Já baixou o App B3").wait_for(state="visible", timeout=5000)
                page.keyboard.press("Escape")
                page.wait_for_timeout(500)
            except Exception:
                pass
            # Ensure we are on www (the SPA host) — navigate if needed
            if "www.investidor.b3.com.br" not in page.url:
                self._goto_with_fallback(
                    page, config.portal_url, timeout_ms=config.timeout_ms, allow_http_error=True
                )
                try:
                    page.wait_for_load_state("networkidle", timeout=10000)
                except Exception:
                    pass
            self.session_file.parent.mkdir(parents=True, exist_ok=True)
            context.storage_state(path=str(self.session_file))
        except TimeoutError as exc:
            self._dump_debug_context(page, reason="auto-login-no-redirect")
            raise SessionExpiredError("Auto-login failed — did not redirect to portfolio.") from exc

    def _open_positions_page(self, page: "Page") -> None:
        from playwright.sync_api import TimeoutError

        for attempt in range(4):
            try:
                self._goto_with_fallback(
                    page,
                    config.portal_url + config.positions_path,
                    timeout_ms=config.timeout_ms,
                    allow_http_error=True,
                )
                try:
                    page.wait_for_load_state("networkidle", timeout=15000)
                except Exception:
                    pass
                page.wait_for_timeout(2000)

                if self._requires_login(page):
                    if attempt < 3:
                        page.wait_for_timeout(5000)
                        continue
                    self._dump_debug_context(page, reason="positions-requires-login")
                    raise SessionExpiredError("B3 login required to refresh session.")

                # The B3 SPA sometimes redirects back to the homepage before the
                # auth state is fully settled. Detect this and retry.
                if config.positions_path not in page.url:
                    if attempt < 3:
                        page.wait_for_timeout(5000)
                        continue
                    self._dump_debug_context(page, reason="positions-redirected-away")
                    raise RuntimeError(
                        f"B3 SPA redirected away from positions page (landed on {page.url!r})."
                    )

                return
            except TimeoutError as exc:
                raise RuntimeError("Timed out while loading the B3 custody page.") from exc

    def _download_file_if_available(self, page: "Page") -> Path | None:
        from playwright.sync_api import Error, TimeoutError

        try:
            # Wait for positions page to fully render before looking for BAIXAR
            try:
                page.wait_for_load_state("networkidle", timeout=15000)
            except Exception:
                pass
            page.wait_for_timeout(2000)

            # Click the BAIXAR button to open the download drawer
            page.locator("text=BAIXAR").first.wait_for(state="visible", timeout=20000)
            page.locator("text=BAIXAR").first.click()

            # Select "Arquivo em Excel" in the drawer
            page.locator("text=Arquivo em Excel").first.wait_for(state="visible", timeout=20000)
            page.locator("text=Arquivo em Excel").first.click()

            # Trigger the download — the drawer's BAIXAR is the last one
            page.wait_for_timeout(1000)
            with page.expect_download(timeout=30000) as download_info:
                page.locator("text=BAIXAR").last.click(force=True)
            download = download_info.value
            path = self.download_dir / ("portfolio" + Path(download.suggested_filename).suffix)
            download.save_as(str(path))
            return path
        except (TimeoutError, Error):
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
            self._dump_debug_context(page, reason="no-table-found")
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

    def _dump_debug_context(self, page: "Page", *, reason: str) -> None:
        """Print diagnostic info to stderr so it surfaces in Railway logs.

        Also writes a screenshot + HTML dump into the download dir for local
        runs, but the stderr output is what matters on Railway where the
        container filesystem isn't easily accessible.
        """
        import sys

        def log(msg: str) -> None:
            print(f"[b3-debug] {msg}", file=sys.stderr)

        log(f"reason={reason}")
        try:
            log(f"url={page.url}")
        except Exception as exc:
            log(f"url=<error: {exc}>")
        try:
            log(f"title={page.title()!r}")
        except Exception as exc:
            log(f"title=<error: {exc}>")
        try:
            html = page.content()
            log(f"html_length={len(html)}")
            # Heuristic markers so we can tell what page we're actually on.
            markers = {
                "cloudflare": "cloudflare" in html.lower() or "cf-chl" in html.lower(),
                "challenge": "just a moment" in html.lower() or "checking your browser" in html.lower(),
                "cpf_input": "placeholder=\"CPF\"" in html or "CPF" in html,
                "password_input": "type=\"password\"" in html,
                "baixar_button": "BAIXAR" in html or "Baixar" in html,
                "posicao_header": "Posição" in html or "posicao" in html.lower(),
                "custody_table": "custody" in html.lower(),
            }
            log(f"markers={markers}")
            # Print the visible text (first 500 chars) so we can tell what the
            # user would see. Falls back silently if body_text() is unavailable.
            try:
                body_text = page.locator("body").inner_text(timeout=2000).strip()
                log(f"body_text_preview={body_text[:500]!r}")
            except Exception:
                pass
        except Exception as exc:
            log(f"html=<error: {exc}>")

        try:
            shot = self.download_dir / "b3_debug_screenshot.png"
            dump = self.download_dir / "b3_debug_page.html"
            page.screenshot(path=str(shot), full_page=True)
            dump.write_text(page.content(), encoding="utf-8")
            log(f"screenshot={shot} html_dump={dump}")
        except Exception as exc:
            log(f"dump_failed={exc}")

    def _requires_login(self, page: "Page") -> bool:
        if "/login" in page.url:
            return True
        # Check for the actual CPF login input rather than generic text,
        # since the word "login" appears in nav menus on authenticated pages too.
        try:
            return page.locator("input[placeholder*='CPF' i]").count() > 0
        except Exception:
            return False

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
