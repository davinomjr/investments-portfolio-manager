from pathlib import Path
from typing import List, Optional

from pydantic_settings import BaseSettings, SettingsConfigDict


ROOT_DIR = Path(__file__).resolve().parents[2]
PROJECT_DIR = ROOT_DIR.parent


class Settings(BaseSettings):
    app_name: str = "Portfolio Manager API"
    database_url: str = f"sqlite:///{PROJECT_DIR / 'database' / 'portfolio.db'}"
    worker_dir: Path = PROJECT_DIR / "worker"
    upload_dir: Path = PROJECT_DIR / "backend" / "uploads"
    data_cache_dir: Path = PROJECT_DIR / "backend" / "data-cache"
    worker_python: str = "python"
    worker_module: str = "app.main"
    worker_command: Optional[str] = None
    default_user_id: int = 1
    cors_origins: List[str] = ["http://localhost:3000"]
    cors_origin_regex: str = r"https?://(localhost|127\.0\.0\.1)(:\d+)?$"
    cvm_itr_base_url: str = "https://dados.cvm.gov.br/dados/cia_aberta/DOC/ITR/DADOS"

    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")


settings = Settings()
