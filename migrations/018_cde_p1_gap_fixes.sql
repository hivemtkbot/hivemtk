-- ============================================================================
-- C/D/E 域 P1 缺口修复迁移 (C/D/E Domain P1 Gap Fixes Migration)
-- 创建时间: 2026-07-21
-- 目的:
--   1. rag_query_logs 新增 top_similarity / hit_in_top1 字段（召回率监控）
--   2. 新建 rag_recall_monitor_snapshots 表（监控指标快照）
--   3. 新建 rag_safety_audit_logs 表（风控卫士审计）
--   4. 新建 sms_number_portability_logs 表（携号转网追踪）
--   5. 邮件退订 / 打开追踪辅助索引
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. rag_query_logs 字段补全
-- ----------------------------------------------------------------------------
ALTER TABLE rag_query_logs ADD COLUMN IF NOT EXISTS top_similarity NUMERIC(10,6) NOT NULL DEFAULT 0;
ALTER TABLE rag_query_logs ADD COLUMN IF NOT EXISTS hit_in_top1    BOOLEAN        NOT NULL DEFAULT FALSE;
ALTER TABLE rag_query_logs ADD COLUMN IF NOT EXISTS top1_doc_id    VARCHAR(128);
CREATE INDEX IF NOT EXISTS idx_rag_query_logs_hit_top1 ON rag_query_logs (hit_in_top1) WHERE hit_in_top1 = TRUE;
CREATE INDEX IF NOT EXISTS idx_rag_query_logs_top_sim  ON rag_query_logs (top_similarity DESC);

-- ----------------------------------------------------------------------------
-- 2. rag_recall_monitor_snapshots 监控快照表
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rag_recall_monitor_snapshots (
    id                BIGSERIAL PRIMARY KEY,
    window_start      TIMESTAMPTZ NOT NULL,
    window_end        TIMESTAMPTZ NOT NULL,
    total_queries     BIGINT      NOT NULL DEFAULT 0,
    top_k_hit_rate    NUMERIC(10,6) NOT NULL DEFAULT 0,
    top_1_hit_rate    NUMERIC(10,6) NOT NULL DEFAULT 0,
    avg_recall        NUMERIC(10,6) NOT NULL DEFAULT 0,
    avg_precision     NUMERIC(10,6) NOT NULL DEFAULT 0,
    avg_similarity    NUMERIC(10,6) NOT NULL DEFAULT 0,
    avg_latency_ms    NUMERIC(12,2) NOT NULL DEFAULT 0,
    p95_latency_ms    BIGINT        NOT NULL DEFAULT 0,
    zero_hit_count    BIGINT        NOT NULL DEFAULT 0,
    low_recall_count  BIGINT        NOT NULL DEFAULT 0,
    payload           TEXT          NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rag_recall_monitor_window ON rag_recall_monitor_snapshots (window_start DESC);

-- ----------------------------------------------------------------------------
-- 3. rag_safety_audit_logs 风控审计表
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rag_safety_audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    user_id     VARCHAR(64),
    tenant_id   VARCHAR(64),
    stage       VARCHAR(16)  NOT NULL, -- input / output / retrieval
    content_hash VARCHAR(64) NOT NULL,  -- SHA256(content) 避免存储原文
    blocked     BOOLEAN      NOT NULL DEFAULT FALSE,
    issues      JSONB        NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rag_safety_audit_tenant ON rag_safety_audit_logs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rag_safety_audit_blocked ON rag_safety_audit_logs (blocked) WHERE blocked = TRUE;

-- ----------------------------------------------------------------------------
-- 4. sms_number_portability_logs 携号转网追踪
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sms_number_portability_logs (
    id              BIGSERIAL PRIMARY KEY,
    phone           VARCHAR(20)  NOT NULL,
    original_carrier VARCHAR(32),  -- 移动 / 联通 / 电信
    current_carrier  VARCHAR(32),  -- 转网后运营商
    detected_at     TIMESTAMPTZ  NOT NULL,
    source          VARCHAR(32)  NOT NULL DEFAULT 'webhook',  -- webhook / manual / sync
    raw_payload     JSONB,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sms_np_phone ON sms_number_portability_logs (phone);
CREATE INDEX IF NOT EXISTS idx_sms_np_detected_at ON sms_number_portability_logs (detected_at DESC);

-- ----------------------------------------------------------------------------
-- 5. 邮件退订 / 打开追踪辅助索引
-- ----------------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_email_unsubscribes_email ON email_unsubscribes (email);
CREATE INDEX IF NOT EXISTS idx_email_tracking_events_email_open ON email_tracking_events (email, event_type) WHERE event_type = 'open';
CREATE INDEX IF NOT EXISTS idx_sms_delivery_statuses_provider_status ON sms_delivery_statuses (provider, status);
