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
        """Launch a persistent-profile browser context with stealth settings.

        Uses launch_persistent_context so cookies, localStorage, and device
        fingerprint survive across runs. B3 uses Azure B2C which recognizes
        the device by these cookies — reusing the profile skips the email
        2FA challenge on subsequent syncs. Auto-login from .env still runs
        each time and is idempotent if the session is already valid.
        """
        config.profile_dir.mkdir(parents=True, exist_ok=True)
        context = playwright.chromium.launch_persistent_context(
            user_data_dir=str(config.profile_dir),
            headless=headless,
            args=_STEALTH_ARGS,
            user_agent=_STEALTH_UA,
            viewport={"width": 1920, "height": 1080},
            accept_downloads=accept_downloads,
            ignore_https_errors=True,
            locale="pt-BR",
            timezone_id="America/Sao_Paulo",
        )
        page = context.pages[0] if context.pages else context.new_page()
        page.add_init_script("Object.defineProperty(navigator, 'webdriver', {get: () => undefined})")
        return context, page

    def bootstrap_login(self) -> Path:
        from playwright.sync_api import sync_playwright

        self.session_file.parent.mkdir(parents=True, exist_ok=True)
        with sync_playwright() as playwright:
            context, page = self._new_context(playwright, headless=False)
            # For manual login, allow Cloudflare/challenge pages to load even if the
            # initial navigation reports an HTTP response error at the network layer.
            self._goto_with_fallback(
                page,
                config.portal_url,
                timeout_ms=config.login_timeout_ms,
                allow_http_error=True,
            )
            page.wait_for_timeout(config.login_timeout_ms)
            # Persistent context saves cookies/localStorage automatically on
            # close; also dump a portable snapshot for reference.
            context.storage_state(path=str(self.session_file))
            context.close()
        return self.session_file

    def import_portfolio(self) -> ImportResult:
        from playwright.sync_api import sync_playwright

        self.download_dir.mkdir(parents=True, exist_ok=True)
        with sync_playwright() as playwright:
            context, page = self._new_context(
                playwright, headless=config.headless, accept_downloads=True,
            )
            try:
                # Navigate to dashboard; B3 is a React SPA that returns 404 at
                # the network layer for all routes, so we allow HTTP errors.
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
                        page = self._auto_login(page, context)
                        # Wait for the SPA auth state to fully settle before navigating
                        page.wait_for_timeout(6000)
                    else:
                        raise SessionExpiredError("B3 login required to refresh session.")

                holdings = self._load_holdings(context, page)
            finally:
                # Always close the context cleanly, even on exception, so the
                # persistent profile flushes cookies (including B3's device-
                # recognition cookie) to disk. Otherwise the next run looks
                # like a fresh device and 2FA gets triggered again.
                try:
                    context.close()
                except Exception:
                    pass
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
        holdings = self._scrape_table(page)
        if not holdings:
            self._dump_debug_context(page, reason="scrape-empty")
        return holdings

    def _fetch_otp_from_gmail(self, since_ts: float):
        """Poll Gmail via IMAP for the freshest B3 2FA email and extract the 6-digit code.

        Returns the code string, or None if nothing matching was found. Requires
        B3_IMAP_USER and B3_IMAP_APP_PASSWORD in the environment (a Gmail app
        password — spaces are stripped so either format works).
        """
        import imaplib
        import email as _email
        import email.policy as _epol
        from email.utils import parsedate_to_datetime
        import os as _os
        import re as _re
        import sys as _sys
        import time as _time
        from html import unescape

        user = (_os.environ.get("B3_IMAP_USER") or "").strip()
        pw = (_os.environ.get("B3_IMAP_APP_PASSWORD") or "").replace(" ", "").strip()
        if not user or not pw:
            return None

        try:
            conn = imaplib.IMAP4_SSL("imap.gmail.com", 993, timeout=15)
        except Exception as exc:
            print(f"[b3-2fa-imap] connect failed: {type(exc).__name__}: {exc}", file=_sys.stderr, flush=True)
            return None
        try:
            conn.login(user, pw)
            conn.select("INBOX")
            # IMAP SINCE is date-only; use yesterday to include emails just before midnight UTC.
            since_str = _time.strftime("%d-%b-%Y", _time.gmtime(since_ts - 86400))
            typ, data = conn.search(
                None,
                f'(FROM "noreply@b3.com.br" SINCE "{since_str}")',
            )
            if typ != "OK" or not data or not data[0]:
                return None
            ids = data[0].split()
            for uid in reversed(ids[-20:]):
                typ, msg_data = conn.fetch(uid, "(RFC822)")
                if typ != "OK" or not msg_data or not msg_data[0]:
                    continue
                raw = msg_data[0][1]
                msg = _email.message_from_bytes(raw, policy=_epol.default)
                try:
                    dt = parsedate_to_datetime(msg.get("Date") or "")
                    if dt and dt.timestamp() < since_ts - 60:
                        continue
                except Exception:
                    pass
                body_text = ""
                body_html = ""
                if msg.is_multipart():
                    for part in msg.walk():
                        ctype = part.get_content_type()
                        if ctype == "text/plain" and not body_text:
                            try:
                                body_text = part.get_content()
                            except Exception:
                                pass
                        elif ctype == "text/html" and not body_html:
                            try:
                                body_html = part.get_content()
                            except Exception:
                                pass
                else:
                    try:
                        body_text = msg.get_content()
                    except Exception:
                        pass
                haystack = body_text or unescape(_re.sub(r"<[^>]+>", " ", body_html or ""))
                m = _re.search(r"Código de segurança[^\d]{0,40}(\d{6})", haystack, _re.IGNORECASE)
                if not m:
                    m = _re.search(r"(?<!\d)(\d{6})(?!\d)", haystack)
                if m:
                    subj = msg.get("Subject") or ""
                    print(
                        f"[b3-2fa-imap] matched code in email subj={subj!r} uid={uid.decode() if isinstance(uid, bytes) else uid}",
                        file=_sys.stderr,
                        flush=True,
                    )
                    return m.group(1)
            return None
        except Exception as exc:
            print(f"[b3-2fa-imap] fetch error: {type(exc).__name__}: {exc}", file=_sys.stderr, flush=True)
            return None
        finally:
            try:
                conn.logout()
            except Exception:
                pass

    def _auto_login(self, page: "Page", context: "BrowserContext") -> "Page":
        """Fill CPF and password on the B3 login page automatically."""
        from playwright.sync_api import TimeoutError

        try:
            # Step 1: fill the CPF input. Page renders both a desktop hero
            # input and a mobile-responsive duplicate; pick whichever is
            # actually visible in the current viewport.
            cpf_input = page.locator(
                "input#input-hero-login:visible, "
                "input#documento-mobile:visible, "
                "input[placeholder*='CPF']:visible"
            ).first
            cpf_input.wait_for(state="visible", timeout=10000)
            cpf_input.click()
            cpf_input.press_sequentially(config.b3_cpf, delay=60)

            # Step 2: click the "Entrar" button. Pressing Enter doesn't work
            # because the button is type="button" (not submit) — the form has
            # no native submit handler, only an Angular click handler.
            page.wait_for_timeout(500)
            entrar_btn = page.locator(
                "button[aria-label='Entrar']:visible, "
                "button:has-text('Entrar'):visible"
            ).first
            entrar_btn.wait_for(state="visible", timeout=5000)
            entrar_btn.click()
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
                # Capture the 2FA page so we can see B3's exact markup and
                # refine the "trust device" selector below on the next pass.
                self._dump_debug_context(page, reason="2fa-page", suffix="2fa")

                # Best-effort auto-check of any "trust device" / "don't ask
                # again" toggle. B3 uses varying labels in Portuguese; try
                # several. If none match, we still submit the code normally
                # and 2FA will be prompted again next time.
                trust_selectors = [
                    "label:has-text('Confiar neste dispositivo')",
                    "label:has-text('confiar neste dispositivo')",
                    "label:has-text('Não pedir novamente')",
                    "label:has-text('não pedir novamente')",
                    "label:has-text('Lembrar deste dispositivo')",
                    "label:has-text('Manter conectado')",
                    "input[type='checkbox']:visible",
                ]
                for sel in trust_selectors:
                    try:
                        el = page.locator(sel).first
                        if el.count() > 0 and el.is_visible():
                            el.check() if sel.startswith("input") else el.click()
                            print(f"[b3-2fa] toggled trust-device via selector: {sel}", file=_sys.stderr, flush=True)
                            break
                    except Exception:
                        continue

                code_file = _P("/tmp/b3-2fa-code")
                if code_file.exists():
                    code_file.unlink()
                import os as _os
                imap_enabled = bool(
                    (_os.environ.get("B3_IMAP_USER") or "").strip()
                    and (_os.environ.get("B3_IMAP_APP_PASSWORD") or "").strip()
                )
                imap_start_ts = _t.time()
                next_imap_check = _t.time() + 5  # give B3 a few seconds to send the email
                if imap_enabled:
                    print("[b3-2fa-imap] Gmail auto-fetch enabled", file=_sys.stderr, flush=True)
                print("[b3-2fa] waiting for Gmail auto-fetch or /tmp/b3-2fa-code (max 5 min)…", file=_sys.stderr, flush=True)
                deadline = _t.time() + 300
                code = None
                while _t.time() < deadline:
                    if code_file.exists():
                        code = code_file.read_text().strip()
                        code_file.unlink()
                        print(f"[b3-2fa] using code from /tmp/b3-2fa-code ({len(code)} digits)", file=_sys.stderr, flush=True)
                        break
                    if imap_enabled and _t.time() >= next_imap_check:
                        next_imap_check = _t.time() + 5
                        code = self._fetch_otp_from_gmail(imap_start_ts)
                        if code:
                            break
                    _t.sleep(2)
                if not code:
                    raise SessionExpiredError("2FA code was not obtained in time.")
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
            return page
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

    def _dismiss_cookie_banner(self, page: "Page") -> None:
        """B3 shows a OneTrust cookie banner whose backdrop intercepts clicks
        on the rest of the page. Click the accept button if it's there."""
        for selector in (
            "#onetrust-accept-btn-handler",
            "button#onetrust-accept-btn-handler",
            ".onetrust-close-btn-handler",
        ):
            try:
                btn = page.locator(selector)
                if btn.count() > 0 and btn.first.is_visible():
                    btn.first.click(timeout=3000)
                    page.wait_for_timeout(500)
                    return
            except Exception:
                continue

    def _download_file_if_available(self, page: "Page") -> Path | None:
        from playwright.sync_api import Error, TimeoutError

        try:
            try:
                page.wait_for_load_state("networkidle", timeout=15000)
            except Exception:
                pass
            page.wait_for_timeout(2000)

            self._dismiss_cookie_banner(page)

            page.locator("text=BAIXAR").first.wait_for(state="visible", timeout=20000)
            page.locator("text=BAIXAR").first.click()

            # Select the Excel radio via its <label for="excel"> — clicking the
            # text node inside doesn't toggle the input, leaving the submit
            # BAIXAR disabled.
            excel_label = page.locator("label[for='excel']")
            excel_label.wait_for(state="visible", timeout=20000)
            excel_label.click()

            # The submit BAIXAR has aria-label="Baixar" and lives inside
            # b3-button.b3i-download-carteira__baixar. Wait for it to become
            # enabled (the click above dispatches an Angular form event).
            submit = page.locator(
                "b3-button.b3i-download-carteira__baixar button[type='submit']"
            )
            submit.wait_for(state="visible", timeout=10000)
            page.wait_for_function(
                "el => el && !el.disabled && el.getAttribute('aria-disabled') !== 'true'",
                arg=submit.element_handle(),
                timeout=10000,
            )

            with page.expect_download(timeout=60000) as download_info:
                submit.click()
            download = download_info.value
            path = self.download_dir / ("portfolio" + Path(download.suggested_filename).suffix)
            download.save_as(str(path))
            return path
        except (TimeoutError, Error) as exc:
            import sys
            print(f"[b3-download] failed: {type(exc).__name__}: {exc}", file=sys.stderr)
            # Capture modal state at the moment of failure. The later
            # scrape-empty dump overwrites the same filenames, so use a
            # download-specific suffix to preserve evidence.
            self._dump_debug_context(page, reason="download-failed", suffix="download")
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

    def _dump_debug_context(self, page: "Page", *, reason: str, suffix: str = "") -> None:
        """Print diagnostic info to stderr so it surfaces in Railway logs.

        Also writes a screenshot + HTML dump into the download dir for local
        runs, but the stderr output is what matters on Railway where the
        container filesystem isn't easily accessible.

        ``suffix`` distinguishes dumps from different failure points in the
        same run (e.g. "download" vs the default scrape-empty dump), so they
        don't clobber each other on disk.
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
            tag = f"_{suffix}" if suffix else ""
            shot = self.download_dir / f"b3_debug_screenshot{tag}.png"
            dump = self.download_dir / f"b3_debug_page{tag}.html"
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
