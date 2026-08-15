-- A/B 测试模块迁移脚本
-- 版本: 1.1.0
-- 适用于: PostgreSQL 15+ (项目唯一数据库)

CREATE TABLE IF NOT EXISTS ab_experiments (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    status VARCHAR(20) DEFAULT 'draft',  -- draft, running, paused, completed
    source_type VARCHAR(50),
    source_id VARCHAR(100),
    traffic_split INTEGER DEFAULT 50,
    start_date TIMESTAMP,
    end_date TIMESTAMP,
    created_by BIGINT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ab_experiments_status ON ab_experiments(status);

CREATE TABLE IF NOT EXISTS ab_variants (
    id BIGSERIAL PRIMARY KEY,
    experiment_id BIGINT NOT NULL,
    name VARCHAR(50) NOT NULL,
    is_control BOOLEAN DEFAULT FALSE,
    config TEXT,
    weight INTEGER DEFAULT 50,
    traffic_count INTEGER DEFAULT 0,
    conversion_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ab_variants_experiment ON ab_variants(experiment_id);

CREATE TABLE IF NOT EXISTS ab_conversion_events (
    id BIGSERIAL PRIMARY KEY,
    experiment_id BIGINT NOT NULL,
    event_name VARCHAR(100) NOT NULL,
    event_type VARCHAR(50),
    event_value DECIMAL(10,2),
    user_id VARCHAR(100),
    variant_id BIGINT,
    metadata TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ab_conversion_events_experiment ON ab_conversion_events(experiment_id);
CREATE INDEX IF NOT EXISTS idx_ab_conversion_events_variant ON ab_conversion_events(variant_id);
CREATE INDEX IF NOT EXISTS idx_ab_conversion_events_user ON ab_conversion_events(user_id);

CREATE TABLE IF NOT EXISTS ab_experiment_results (
    id BIGSERIAL PRIMARY KEY,
    experiment_id BIGINT NOT NULL,
    variant_id BIGINT NOT NULL,
    variant_name VARCHAR(50),
    is_control BOOLEAN,
    traffic_count INTEGER,
    conversion_count INTEGER,
    conversion_rate DECIMAL(10,2),
    revenue DECIMAL(10,2),
    average_value DECIMAL(10,2),
    confidence_level DECIMAL(5,2),
    is_winner BOOLEAN DEFAULT FALSE,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ab_experiment_results_exp_variant ON ab_experiment_results(experiment_id, variant_id);
CREATE INDEX IF NOT EXISTS idx_ab_experiment_results_winner ON ab_experiment_results(is_winner);

CREATE OR REPLACE FUNCTION ab_experiments_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_ab_experiments_updated_at ON ab_experiments;
CREATE TRIGGER trg_ab_experiments_updated_at
    BEFORE UPDATE ON ab_experiments
    FOR EACH ROW EXECUTE FUNCTION ab_experiments_set_updated_at();

CREATE OR REPLACE FUNCTION ab_variants_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_ab_variants_updated_at ON ab_variants;
CREATE TRIGGER trg_ab_variants_updated_at
    BEFORE UPDATE ON ab_variants
    FOR EACH ROW EXECUTE FUNCTION ab_variants_set_updated_at();

CREATE OR REPLACE FUNCTION ab_experiment_results_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_ab_experiment_results_updated_at ON ab_experiment_results;
CREATE TRIGGER trg_ab_experiment_results_updated_at
    BEFORE UPDATE ON ab_experiment_results
    FOR EACH ROW EXECUTE FUNCTION ab_experiment_results_set_updated_at();
