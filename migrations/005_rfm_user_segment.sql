-- 用户分层 RFM 数据库迁移脚本
-- 版本: 1.1.0
-- 适用于: PostgreSQL 15+ (项目唯一数据库)

-- RFM 规则表
CREATE TABLE IF NOT EXISTS rfm_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100),
    r_days_1 INT DEFAULT 7,
    r_days_2 INT DEFAULT 14,
    r_days_3 INT DEFAULT 30,
    r_days_4 INT DEFAULT 60,
    r_days_5 INT DEFAULT 90,
    f_count_1 INT DEFAULT 1,
    f_count_2 INT DEFAULT 3,
    f_count_3 INT DEFAULT 5,
    f_count_4 INT DEFAULT 10,
    f_count_5 INT DEFAULT 20,
    m_amount_1 DECIMAL(10,2) DEFAULT 100,
    m_amount_2 DECIMAL(10,2) DEFAULT 500,
    m_amount_3 DECIMAL(10,2) DEFAULT 1000,
    m_amount_4 DECIMAL(10,2) DEFAULT 5000,
    m_amount_5 DECIMAL(10,2) DEFAULT 10000,
    is_active BOOLEAN DEFAULT TRUE,
    is_system BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_rfm_rules_is_active ON rfm_rules (is_active);
CREATE INDEX IF NOT EXISTS idx_rfm_rules_is_system ON rfm_rules (is_system);
COMMENT ON COLUMN rfm_rules.name IS '规则名称';
COMMENT ON COLUMN rfm_rules.r_days_1 IS 'R 值<= ？天，得 5 分';
COMMENT ON COLUMN rfm_rules.r_days_2 IS 'R 值<= ？天，得 4 分';
COMMENT ON COLUMN rfm_rules.r_days_3 IS 'R 值<= ？天，得 3 分';
COMMENT ON COLUMN rfm_rules.r_days_4 IS 'R 值<= ？天，得 2 分';
COMMENT ON COLUMN rfm_rules.r_days_5 IS 'R 值<= ？天，得 1 分';
COMMENT ON COLUMN rfm_rules.f_count_1 IS '消费次数>= ？，得 1 分';
COMMENT ON COLUMN rfm_rules.f_count_2 IS '消费次数>= ？，得 2 分';
COMMENT ON COLUMN rfm_rules.f_count_3 IS '消费次数>= ？，得 3 分';
COMMENT ON COLUMN rfm_rules.f_count_4 IS '消费次数>= ？，得 4 分';
COMMENT ON COLUMN rfm_rules.f_count_5 IS '消费次数>= ？，得 5 分';
COMMENT ON COLUMN rfm_rules.m_amount_1 IS '消费金额>= ？，得 1 分';
COMMENT ON COLUMN rfm_rules.m_amount_2 IS '消费金额>= ？，得 2 分';
COMMENT ON COLUMN rfm_rules.m_amount_3 IS '消费金额>= ？，得 3 分';
COMMENT ON COLUMN rfm_rules.m_amount_4 IS '消费金额>= ？，得 4 分';
COMMENT ON COLUMN rfm_rules.m_amount_5 IS '消费金额>= ？，得 5 分';
COMMENT ON COLUMN rfm_rules.is_active IS '是否活跃';
COMMENT ON COLUMN rfm_rules.is_system IS '是否系统内置';
COMMENT ON TABLE rfm_rules IS 'RFM 规则表';

-- updated_at 自动更新触发器 (rfm_rules)
CREATE OR REPLACE FUNCTION rfm_rules_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_rfm_rules_updated_at ON rfm_rules;
CREATE TRIGGER trg_rfm_rules_updated_at
    BEFORE UPDATE ON rfm_rules
    FOR EACH ROW EXECUTE FUNCTION rfm_rules_set_updated_at();

-- 用户 RFM 表
CREATE TABLE IF NOT EXISTS user_rfms (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    r_score INT DEFAULT 0,
    f_score INT DEFAULT 0,
    m_score INT DEFAULT 0,
    total_score INT DEFAULT 0,
    layer VARCHAR(20),
    last_transaction_at TIMESTAMP,
    transaction_count INT DEFAULT 0,
    total_amount DECIMAL(10,2) DEFAULT 0,
    avg_amount DECIMAL(10,2) DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_user_rfms_user ON user_rfms (user_id);
CREATE INDEX IF NOT EXISTS idx_user_rfms_layer ON user_rfms (layer);
CREATE INDEX IF NOT EXISTS idx_user_rfms_total_score ON user_rfms (total_score);
COMMENT ON COLUMN user_rfms.layer IS '用户分层';
COMMENT ON COLUMN user_rfms.last_transaction_at IS '最后交易时间';
COMMENT ON COLUMN user_rfms.transaction_count IS '交易次数';
COMMENT ON COLUMN user_rfms.total_amount IS '总消费金额';
COMMENT ON COLUMN user_rfms.avg_amount IS '平均消费金额';
COMMENT ON TABLE user_rfms IS '用户 RFM 表';

-- updated_at 自动更新触发器 (user_rfms)
CREATE OR REPLACE FUNCTION user_rfms_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_user_rfms_updated_at ON user_rfms;
CREATE TRIGGER trg_user_rfms_updated_at
    BEFORE UPDATE ON user_rfms
    FOR EACH ROW EXECUTE FUNCTION user_rfms_set_updated_at();

-- 初始化默认 RFM 规则 (使用 ON CONFLICT 处理幂等, 按 name 唯一)
CREATE UNIQUE INDEX IF NOT EXISTS uq_rfm_rules_name ON rfm_rules (name) WHERE name IS NOT NULL;
INSERT INTO rfm_rules (name, is_system, is_active) VALUES
    ('默认 RFM 规则', TRUE, TRUE)
ON CONFLICT (name) DO UPDATE SET
    is_system = EXCLUDED.is_system,
    is_active = EXCLUDED.is_active;
