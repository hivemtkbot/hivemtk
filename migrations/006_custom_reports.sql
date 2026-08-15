-- 自定义报表数据库迁移脚本
-- 版本: 1.1.0
-- 适用于: PostgreSQL 15+ (项目唯一数据库)

CREATE TABLE IF NOT EXISTS custom_reports (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    data_source VARCHAR(50) NOT NULL,
    dimensions TEXT,
    metrics TEXT,
    filters TEXT,
    chart_type VARCHAR(20),
    chart_config TEXT,
    is_public BOOLEAN DEFAULT FALSE,
    created_by BIGINT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_custom_reports_data_source ON custom_reports (data_source);
CREATE INDEX IF NOT EXISTS idx_custom_reports_public ON custom_reports (is_public);
CREATE INDEX IF NOT EXISTS idx_custom_reports_created_by ON custom_reports (created_by);
COMMENT ON COLUMN custom_reports.name IS '报表名称';
COMMENT ON COLUMN custom_reports.description IS '报表描述';
COMMENT ON COLUMN custom_reports.data_source IS '数据源：sessions, messages, orders, clues, users, rfm, agents';
COMMENT ON COLUMN custom_reports.dimensions IS '维度配置 (JSON)';
COMMENT ON COLUMN custom_reports.metrics IS '指标配置 (JSON)';
COMMENT ON COLUMN custom_reports.filters IS '筛选条件 (JSON)';
COMMENT ON COLUMN custom_reports.chart_type IS '图表类型：table, line, bar, pie, area, card';
COMMENT ON COLUMN custom_reports.chart_config IS '图表配置 (JSON)';
COMMENT ON COLUMN custom_reports.is_public IS '是否公开';
COMMENT ON COLUMN custom_reports.created_by IS '创建人 ID';
COMMENT ON TABLE custom_reports IS '自定义报表表';

CREATE OR REPLACE FUNCTION custom_reports_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_custom_reports_updated_at ON custom_reports;
CREATE TRIGGER trg_custom_reports_updated_at
    BEFORE UPDATE ON custom_reports
    FOR EACH ROW EXECUTE FUNCTION custom_reports_set_updated_at();

-- 初始化默认报表模板 (使用 ON CONFLICT 处理幂等, 按 name 唯一)
CREATE UNIQUE INDEX IF NOT EXISTS uq_custom_reports_name ON custom_reports (name);
INSERT INTO custom_reports (name, description, data_source, dimensions, metrics, chart_type, is_public) VALUES
    ('会话趋势分析', '客服会话数量趋势分析', 'sessions',
     '[{"field": "date", "label": "日期", "data_type": "date"}]',
     '[{"field": "session_count", "label": "会话数", "agg_func": "count"}, {"field": "avg_duration", "label": "平均时长", "agg_func": "avg"}]',
     'line', TRUE),
    ('消息类型分布', '不同类型消息占比分析', 'messages',
     '[{"field": "msg_type", "label": "消息类型", "data_type": "string"}]',
     '[{"field": "message_count", "label": "消息数", "agg_func": "count"}]',
     'pie', TRUE),
    ('客服绩效排行', '客服工作量统计', 'agents',
     '[{"field": "agent_name", "label": "客服姓名", "data_type": "string"}]',
     '[{"field": "session_count", "label": "会话数", "agg_func": "count"}, {"field": "avg_response_time", "label": "平均响应时间", "agg_func": "avg"}]',
     'bar', TRUE),
    ('用户分层统计', 'RFM 用户分层分布', 'rfm',
     '[{"field": "layer", "label": "用户分层", "data_type": "string"}]',
     '[{"field": "user_count", "label": "用户数", "agg_func": "count"}]',
     'pie', TRUE)
ON CONFLICT (name) DO UPDATE SET
    description = EXCLUDED.description,
    data_source = EXCLUDED.data_source,
    dimensions = EXCLUDED.dimensions,
    metrics = EXCLUDED.metrics,
    chart_type = EXCLUDED.chart_type,
    is_public = EXCLUDED.is_public;
