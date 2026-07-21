-- 流失预警模块迁移脚本
-- 版本: 1.1.0  (2026-07-17 改为 PostgreSQL 15+ 语法)
-- 适用于: PostgreSQL 15+ (项目唯一数据库)

-- 流失预测表
CREATE TABLE IF NOT EXISTS churn_predictions (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(100) NOT NULL,
    churn_score DECIMAL(10,2),
    churn_risk VARCHAR(20),  -- low, medium, high, critical
    risk_factors TEXT,  -- JSON 数组
    last_activity_at TIMESTAMP,
    last_purchase_at TIMESTAMP,
    days_since_active INTEGER,
    purchase_freq DECIMAL(10,2),
    average_order_value DECIMAL(10,2),
    predicted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_churn_predictions_user ON churn_predictions(user_id);
CREATE INDEX IF NOT EXISTS idx_churn_predictions_risk ON churn_predictions(churn_risk);
CREATE INDEX IF NOT EXISTS idx_churn_predictions_score ON churn_predictions(churn_score DESC);

-- 流失预警表
CREATE TABLE IF NOT EXISTS churn_warnings (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(100) NOT NULL,
    warning_level VARCHAR(20),  -- low, medium, high, critical
    warning_type VARCHAR(50),
    description VARCHAR(500),
    suggestion TEXT,
    is_handled BOOLEAN DEFAULT FALSE,
    handled_at TIMESTAMP,
    handled_by BIGINT,
    handled_note TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_churn_warnings_user ON churn_warnings(user_id);
CREATE INDEX IF NOT EXISTS idx_churn_warnings_handled ON churn_warnings(is_handled);
CREATE INDEX IF NOT EXISTS idx_churn_warnings_level ON churn_warnings(warning_level);

-- 流失模型配置表
CREATE TABLE IF NOT EXISTS churn_model_configs (
    id BIGSERIAL PRIMARY KEY,
    inactive_days_weight DECIMAL(5,2) DEFAULT 0.3,
    purchase_freq_weight DECIMAL(5,2) DEFAULT 0.3,
    order_value_weight DECIMAL(5,2) DEFAULT 0.2,
    engagement_weight DECIMAL(5,2) DEFAULT 0.2,
    inactive_threshold INTEGER DEFAULT 30,
    purchase_threshold INTEGER DEFAULT 60,
    high_risk_score DECIMAL(5,2) DEFAULT 70,
    critical_risk_score DECIMAL(5,2) DEFAULT 85,
    last_calculated_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 流失统计表
CREATE TABLE IF NOT EXISTS churn_statistics (
    id BIGSERIAL PRIMARY KEY,
    stat_date VARCHAR(20) NOT NULL,  -- YYYY-MM-DD
    total_users INTEGER,
    churn_users INTEGER,
    churn_rate DECIMAL(10,2),
    high_risk_users INTEGER,
    critical_users INTEGER,
    recovered_users INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_churn_statistics_date ON churn_statistics(stat_date);

-- updated_at 自动更新触发器
CREATE OR REPLACE FUNCTION churn_predictions_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_churn_predictions_updated_at ON churn_predictions;
CREATE TRIGGER trg_churn_predictions_updated_at
    BEFORE UPDATE ON churn_predictions
    FOR EACH ROW EXECUTE FUNCTION churn_predictions_set_updated_at();

CREATE OR REPLACE FUNCTION churn_warnings_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_churn_warnings_updated_at ON churn_warnings;
CREATE TRIGGER trg_churn_warnings_updated_at
    BEFORE UPDATE ON churn_warnings
    FOR EACH ROW EXECUTE FUNCTION churn_warnings_set_updated_at();

CREATE OR REPLACE FUNCTION churn_model_configs_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_churn_model_configs_updated_at ON churn_model_configs;
CREATE TRIGGER trg_churn_model_configs_updated_at
    BEFORE UPDATE ON churn_model_configs
    FOR EACH ROW EXECUTE FUNCTION churn_model_configs_set_updated_at();
