-- USR-WB-03: 客服自动分配规则
CREATE TABLE IF NOT EXISTS assignment_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    strategy VARCHAR(30) NOT NULL, -- 'round_robin' | 'least_busy' | 'skill_match' | 'manual'
    conditions JSONB, -- {channel, language, skill, customer_tier, time_window}
    skill_match_rules JSONB, -- skill_match 策略下：{required_skill, weight}
    priority INT DEFAULT 0, -- 数值越大越优先
    enabled BOOLEAN DEFAULT TRUE,
    created_by BIGINT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_assignment_rules_priority ON assignment_rules(enabled, priority DESC) WHERE deleted_at IS NULL;

COMMENT ON TABLE assignment_rules IS '客服会话自动分配规则（USR-WB-03）';

-- USR-WB-04: SLA 策略
CREATE TABLE IF NOT EXISTS sla_policies (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    first_response_seconds INT NOT NULL,
    resolution_seconds INT,
    applies_to JSONB, -- {channel, customer_tier, business_hours}
    warn_threshold INT DEFAULT 80, -- 达 N% 触发告警
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sla_policies_enabled ON sla_policies(enabled);

COMMENT ON TABLE sla_policies IS 'SLA 策略表（USR-WB-04）';

-- USR-WB-04: SLA 违规记录
CREATE TABLE IF NOT EXISTS sla_violations (
    id BIGSERIAL PRIMARY KEY,
    policy_id BIGINT NOT NULL REFERENCES sla_policies(id),
    session_id VARCHAR(50) NOT NULL,
    violation_type VARCHAR(20) NOT NULL, -- 'first_response' | 'resolution'
    sla_seconds INT NOT NULL,
    actual_seconds INT NOT NULL,
    detected_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sla_violations_session ON sla_violations(session_id, violation_type);

COMMENT ON TABLE sla_violations IS 'SLA 违规记录（USR-WB-04）';
