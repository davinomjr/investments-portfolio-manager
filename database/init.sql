CREATE TABLE IF NOT EXISTS assets (
    id SERIAL PRIMARY KEY,
    ticker VARCHAR(32) NOT NULL UNIQUE,
    asset_type VARCHAR(32) NOT NULL,
    currency VARCHAR(8) NOT NULL DEFAULT 'BRL'
);

CREATE TABLE IF NOT EXISTS positions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    asset_id INTEGER NOT NULL REFERENCES assets(id),
    quantity NUMERIC(18, 4) NOT NULL,
    avg_price NUMERIC(18, 4) NOT NULL,
    broker VARCHAR(128),
    source VARCHAR(32) NOT NULL DEFAULT 'b3',
    last_updated TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_positions_user_asset UNIQUE (user_id, asset_id)
);

CREATE TABLE IF NOT EXISTS asset_metadata (
    id SERIAL PRIMARY KEY,
    asset_id INTEGER NOT NULL REFERENCES assets(id),
    company_name VARCHAR(255),
    tax_id VARCHAR(32),
    last_updated TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_asset_metadata_asset UNIQUE (asset_id)
);

CREATE TABLE IF NOT EXISTS import_jobs (
    id SERIAL PRIMARY KEY,
    source VARCHAR(32) NOT NULL DEFAULT 'b3',
    status VARCHAR(32) NOT NULL DEFAULT 'queued',
    detail TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
