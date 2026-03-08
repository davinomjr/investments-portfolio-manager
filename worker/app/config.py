from pathlib import Path

from dataclasses import dataclass


@dataclass
class WorkerConfig:
    # The non-www host currently returns 404. The portal is served behind
    # Cloudflare on the www host.
    portal_url: str = "https://www.investidor.b3.com.br"
    dashboard_path: str = "/"
    positions_path: str = "/custodia"
    login_timeout_ms: int = 120000
    session_file: Path = Path(__file__).resolve().parents[1] / "data" / "b3_session.json"
    download_dir: Path = Path(__file__).resolve().parents[1] / "data" / "downloads"
    headless: bool = False
    timeout_ms: int = 30000


config = WorkerConfig()
