-- ============================================================
-- 资产市场 · 用户端 3 张表
-- 设计文档: docs/architecture/ASSET_MARKET_INTEGRATION.md
-- ============================================================

CREATE TABLE IF NOT EXISTS local_assets (
    id BIGSERIAL PRIMARY KEY,
    asset_id VARCHAR(64) UNIQUE NOT NULL,
    asset_type VARCHAR(32) NOT NULL,
    industry VARCHAR(32) NOT NULL,
    name VARCHAR(128) NOT NULL,
    version VARCHAR(16) NOT NULL,
    source VARCHAR(16) DEFAULT 'purchased',
    is_active BOOLEAN DEFAULT TRUE,
    purchase_id BIGINT,
    purchased_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_local_assets_type ON local_assets(asset_type);
CREATE INDEX IF NOT EXISTS idx_local_assets_industry ON local_assets(industry);
CREATE INDEX IF NOT EXISTS idx_local_assets_active ON local_assets(is_active);
CREATE INDEX IF NOT EXISTS idx_local_assets_source ON local_assets(source);
CREATE INDEX IF NOT EXISTS idx_local_assets_deleted ON local_assets(deleted_at);

CREATE TABLE IF NOT EXISTS local_asset_data (
    id BIGSERIAL PRIMARY KEY,
    local_asset_id BIGINT NOT NULL REFERENCES local_assets(id) ON DELETE CASCADE,
    data JSONB NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(local_asset_id)
);
CREATE INDEX IF NOT EXISTS idx_local_asset_data_gin ON local_asset_data USING GIN(data);

CREATE TABLE IF NOT EXISTS local_asset_sync_log (
    id BIGSERIAL PRIMARY KEY,
    asset_id VARCHAR(64) NOT NULL,
    action VARCHAR(32) NOT NULL,
    status VARCHAR(16) NOT NULL,
    error_msg TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sync_log_asset ON local_asset_sync_log(asset_id);
CREATE INDEX IF NOT EXISTS idx_sync_log_created ON local_asset_sync_log(created_at DESC);
