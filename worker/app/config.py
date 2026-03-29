from pathlib import Path
from dataclasses import dataclass, field
import os


def _load_env_file(path: Path) -> None:
    """Load key=value pairs from a .env file into os.environ (if not already set)."""
    if not path.exists():
        return
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key and key not in os.environ:
            os.environ[key] = value


_load_env_file(Path(__file__).resolve().parents[1] / ".env")


@dataclass
class WorkerConfig:
    # The non-www host currently returns 404. The portal is served behind
    # Cloudflare on the www host.
    portal_url: str = "https://www.investidor.b3.com.br"
    dashboard_path: str = "/"
    positions_path: str = "/minha-carteira/investimentos/posicao"
    login_timeout_ms: int = 120000
    session_file: Path = Path(__file__).resolve().parents[1] / "data" / "b3_session.json"
    download_dir: Path = Path(__file__).resolve().parents[1] / "data" / "downloads"
    headless: bool = field(default_factory=lambda: os.environ.get("HEADLESS", "false").lower() in ("1", "true", "yes"))
    timeout_ms: int = 30000
    b3_cpf: str = field(default_factory=lambda: os.environ.get("B3_CPF", ""))
    b3_password: str = field(default_factory=lambda: os.environ.get("B3_PASSWORD", ""))


config = WorkerConfig()
