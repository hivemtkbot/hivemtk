-- USR-AI-05: Feature Flag 独立模块
-- 借鉴：https://github.com/growthbook/growthbook
-- 与 abExperiment 共享哈希分桶（按 user_id 哈希到 0-9999）
CREATE TABLE IF NOT EXISTS feature_flags (
    id BIGSERIAL PRIMARY KEY,
    key VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(200),
    description TEXT,
    type VARCHAR(20) NOT NULL DEFAULT 'boolean', -- 'boolean' | 'string' | 'number' | 'json'
    default_value JSONB,
    rules JSONB, -- 规则：{attribute, operator, value}
    rollout_percentage INT DEFAULT 0, -- 0-100
    rollout_attributes JSONB, -- 灰度属性（如 user_segment / plan）
    enabled BOOLEAN DEFAULT FALSE,
    created_by BIGINT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_feature_flags_enabled ON feature_flags(enabled) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_feature_flags_key ON feature_flags(key) WHERE deleted_at IS NULL;

-- Feature Flag 评估日志（用于调试 + Stale Flag 检测）
CREATE TABLE IF NOT EXISTS feature_flag_evaluations (
    id BIGSERIAL PRIMARY KEY,
    flag_key VARCHAR(100) NOT NULL,
    user_id VARCHAR(100),
    attributes JSONB,
    value JSONB,
    reason VARCHAR(50), -- 'rollout' | 'rule_match' | 'default' | 'disabled'
    evaluated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ff_eval_key_time ON feature_flag_evaluations(flag_key, evaluated_at DESC);

COMMENT ON TABLE feature_flags IS 'Feature Flag 独立表（USR-AI-05），与 abExperiment 解耦';
COMMENT ON TABLE feature_flag_evaluations IS 'Feature Flag 评估日志（用于调试 + Stale Flag 检测）';
