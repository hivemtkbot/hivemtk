-- 满意度调研模块迁移脚本
-- 010_satisfaction_surveys.sql
-- 版本: 1.1.0
-- 适用于: PostgreSQL 15+ (项目唯一数据库)

CREATE TABLE IF NOT EXISTS satisfaction_surveys (
    id VARCHAR(36) PRIMARY KEY,
    merchant_key VARCHAR(64) NOT NULL,
    merchant_name VARCHAR(255) DEFAULT '',
    rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
    feedback TEXT,
    contact VARCHAR(255) DEFAULT '',
    category VARCHAR(50) DEFAULT 'general',  -- general/feature/support/billing
    user_id VARCHAR(100) DEFAULT '',
    channel VARCHAR(50) DEFAULT 'web',      -- web/email/api
    submitted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_satisfaction_surveys_merchant ON satisfaction_surveys(merchant_key);
CREATE INDEX IF NOT EXISTS idx_satisfaction_surveys_rating ON satisfaction_surveys(rating);
CREATE INDEX IF NOT EXISTS idx_satisfaction_surveys_submitted ON satisfaction_surveys(submitted_at);
CREATE INDEX IF NOT EXISTS idx_satisfaction_surveys_category ON satisfaction_surveys(category);

CREATE TABLE IF NOT EXISTS satisfaction_survey_templates (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(50) NOT NULL,
    questions TEXT NOT NULL,  -- JSON 数组: 问题列表
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_satisfaction_templates_category ON satisfaction_survey_templates(category);
CREATE INDEX IF NOT EXISTS idx_satisfaction_templates_active ON satisfaction_survey_templates(is_active);

CREATE TABLE IF NOT EXISTS satisfaction_followups (
    id VARCHAR(36) PRIMARY KEY,
    survey_id VARCHAR(36) NOT NULL,
    merchant_key VARCHAR(64) NOT NULL,
    assigned_to BIGINT,
    follow_note TEXT,
    status VARCHAR(20) DEFAULT 'pending',  -- pending/in_progress/resolved/closed
    resolved_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_satisfaction_followups_survey ON satisfaction_followups(survey_id);
CREATE INDEX IF NOT EXISTS idx_satisfaction_followups_merchant ON satisfaction_followups(merchant_key);
CREATE INDEX IF NOT EXISTS idx_satisfaction_followups_status ON satisfaction_followups(status);

CREATE OR REPLACE FUNCTION satisfaction_surveys_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_satisfaction_surveys_updated_at ON satisfaction_surveys;
CREATE TRIGGER trg_satisfaction_surveys_updated_at
    BEFORE UPDATE ON satisfaction_surveys
    FOR EACH ROW EXECUTE FUNCTION satisfaction_surveys_set_updated_at();

CREATE OR REPLACE FUNCTION satisfaction_survey_templates_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_satisfaction_survey_templates_updated_at ON satisfaction_survey_templates;
CREATE TRIGGER trg_satisfaction_survey_templates_updated_at
    BEFORE UPDATE ON satisfaction_survey_templates
    FOR EACH ROW EXECUTE FUNCTION satisfaction_survey_templates_set_updated_at();

CREATE OR REPLACE FUNCTION satisfaction_followups_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_satisfaction_followups_updated_at ON satisfaction_followups;
CREATE TRIGGER trg_satisfaction_followups_updated_at
    BEFORE UPDATE ON satisfaction_followups
    FOR EACH ROW EXECUTE FUNCTION satisfaction_followups_set_updated_at();
