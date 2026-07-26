-- ============================================================================
-- Migration 029: 活码点击日志表 (F-P0-19)
-- 目的:
--   1. livecode_click_log 记录活码维度的原始点击事件（访客点击落地页按钮）
--      供 LiveCodeService.GetStats 聚合 TotalClicks / TodayClicks，替代硬编码 0
--   2. qr_code_click_log 记录二维码维度的原始点击事件
--      供 LiveCodeService.GetQRStats 聚合 ViewCount / ClickCount，替代硬编码 0
--   3. 两表均保留 user_agent / referrer / ip_address，便于后续来源分析与风控
-- 关联功能项: F-P0-19 活码统计真实化
-- 关联模型: internal/model/live_code_click_log.go
-- 关联服务: internal/service/live_code.go (RecordClick / GetStats / GetQRStats)
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. livecode_click_log 活码点击日志
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS livecode_click_log (
    id            BIGSERIAL    PRIMARY KEY,
    live_code_id  VARCHAR(36)  NOT NULL,
    qr_code_id    VARCHAR(36),
    user_agent    VARCHAR(500),
    referrer      VARCHAR(500),
    ip_address    VARCHAR(64),
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_livecode_click_log_live_code_id ON livecode_click_log (live_code_id);
CREATE INDEX IF NOT EXISTS idx_livecode_click_log_qr_code_id  ON livecode_click_log (qr_code_id);
CREATE INDEX IF NOT EXISTS idx_livecode_click_log_created_at  ON livecode_click_log (created_at);

COMMENT ON TABLE  livecode_click_log IS 'F-P0-19: 活码点击事件日志，供 GetStats/GetQRStats 真实聚合';
COMMENT ON COLUMN livecode_click_log.live_code_id IS '活码 ID（live_codes.id）';
COMMENT ON COLUMN livecode_click_log.qr_code_id   IS '二维码 ID（live_code_qrs.id），可空（直接点击活码落地页时无二维码）';

-- ----------------------------------------------------------------------------
-- 2. qr_code_click_log 二维码点击日志
--    （与 livecode_click_log 分表存储，便于按二维码维度独立聚合与清理）
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS qr_code_click_log (
    id            BIGSERIAL    PRIMARY KEY,
    qr_code_id    VARCHAR(36)  NOT NULL,
    live_code_id  VARCHAR(36)  NOT NULL,
    user_agent    VARCHAR(500),
    referrer      VARCHAR(500),
    ip_address    VARCHAR(64),
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_qr_code_click_log_qr_code_id   ON qr_code_click_log (qr_code_id);
CREATE INDEX IF NOT EXISTS idx_qr_code_click_log_live_code_id ON qr_code_click_log (live_code_id);
CREATE INDEX IF NOT EXISTS idx_qr_code_click_log_created_at   ON qr_code_click_log (created_at);

COMMENT ON TABLE  qr_code_click_log IS 'F-P0-19: 二维码点击事件日志，供 GetQRStats 真实聚合 ViewCount/ClickCount';
COMMENT ON COLUMN qr_code_click_log.qr_code_id   IS '二维码 ID（live_code_qrs.id）';
COMMENT ON COLUMN qr_code_click_log.live_code_id IS '冗余活码 ID，便于跨表联查与按活码维度清理';
