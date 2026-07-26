-- ============================================================
-- 资产市场种子数据补全（确保 API 端点 id=1 验证可用）
-- 背景:
--   * 之前 local_assets id=1 被软删除（deleted_at 已设置），导致
--     `GET /api/v1/local-assets/1` 返回 6001 资产不存在
--   * platform-server market_assets.id=1 的业务 ID 为 hive_e2e_agent_001
--   * 需建立 user-server 端 local_assets.id=1 与 platform-server
--     market_assets.asset_id=hive_e2e_agent_001 的关联，保证
--     端点测试与回归用例可独立报告错误
-- 设计文档: docs/architecture/ASSET_MARKET_INTEGRATION.md
-- ============================================================

-- 1) 清理已软删的 id=1 记录（重置序列前必须删除）
DELETE FROM local_asset_data WHERE local_asset_id = 1;
DELETE FROM local_assets WHERE id = 1;

-- 2) 重置 local_assets 序列，使 id=1 可被重新分配
SELECT setval(pg_get_serial_sequence('local_assets', 'id'),
              GREATEST(1, (SELECT COALESCE(MAX(id), 0) FROM local_assets) - 1));

-- 3) 重新插入 id=1 记录，asset_id 匹配 platform-server 的 hive_e2e_agent_001
--    字段与模型 LocalAsset 严格对齐
INSERT INTO local_assets (
    id, asset_id, asset_type, industry, name, version, source,
    is_active, use_count, reported_use_count, synced_at, updated_at, created_at
) VALUES (
    1,
    'hive_e2e_agent_001',
    'agent_persona',
    '美妆',
    'E2E测试智能体',
    '1.0.0',
    'purchased',
    TRUE,
    1,
    1,
    NOW(),
    NOW(),
    NOW()
) ON CONFLICT (id) DO NOTHING;

-- 4) 插入 local_asset_data（资产 JSON 数据）
INSERT INTO local_asset_data (local_asset_id, data, updated_at)
SELECT 1, $${"name":"E2E测试智能体","system_prompt":"你是一个专业的美妆销售顾问，负责解答客户关于护肤、彩妆、香水的问题，并基于客户肤质推荐合适产品、促成下单。","greeting_templates":["您好，我是您的美妆顾问，请问今天想了解哪类护肤产品？","很高兴为您服务，请告诉我您的肤质和需求。"],"tone":"professional","industry":"美妆","asset_type":"agent_persona","version":"1.0.0"}$$::jsonb, NOW()
WHERE NOT EXISTS (SELECT 1 FROM local_asset_data WHERE local_asset_id = 1);

-- 5) 插入一条同步日志
INSERT INTO local_asset_sync_log (asset_id, action, status, error_msg, created_at)
VALUES (
    'hive_e2e_agent_001',
    'purchase_sync',
    'success',
    '',
    NOW()
)
ON CONFLICT DO NOTHING;
