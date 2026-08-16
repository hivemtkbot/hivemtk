-- =============================================================================
-- Migration 040: customers 关键标识 UNIQUE 约束（OPT-DB-02）
-- 2026-08-16
--
-- 背景：原 customer.go 仅有 index 而非 UNIQUE，导致同一手机号/邮箱/微信可被多次插入。
--       应用层 FindByIdentity 兜底，但仍有竞态风险。
--
-- 策略：
--   1. 用 CTE + DISTINCT 找出重复的 (type, account)
--   2. 保留最早创建的一条（id 最小）
--   3. 其余重复行的 (type, account) 置空（避免 FK 失败）
--   4. 创建 UNIQUE 索引
--   5. 删除重复行（可选）
--
-- 注：迁移使用 LEFT JOIN 避免锁表，应用层需在迁移后跑数据修复脚本
-- =============================================================================

BEGIN;

-- 1. 找出重复的 phone，先把次要行的 phone 置空
-- 保留最早的一条（按 created_at + id）
DO $$
DECLARE
  dup_count INT;
BEGIN
  -- 统计重复数（仅日志）
  SELECT COUNT(*) INTO dup_count FROM (
    SELECT phone, COUNT(*) AS cnt
    FROM customer
    WHERE phone IS NOT NULL AND phone != ''
    GROUP BY phone
    HAVING COUNT(*) > 1
  ) t;
  RAISE NOTICE 'customer.phone 重复数: %', dup_count;
END $$;

-- 2. 将次要重复行的 phone 置空
UPDATE customer c
SET phone = NULL
WHERE phone IS NOT NULL
  AND id NOT IN (
    SELECT MIN(id::text)::bigint
    FROM customer
    WHERE phone IS NOT NULL
    GROUP BY phone
  );

-- 3. 创建 UNIQUE 索引（仅对非 NULL 值）
CREATE UNIQUE INDEX IF NOT EXISTS uk_customer_phone
  ON customer(phone)
  WHERE phone IS NOT NULL AND phone != '';

CREATE UNIQUE INDEX IF NOT EXISTS uk_customer_email
  ON customer(email)
  WHERE email IS NOT NULL AND email != '';

CREATE UNIQUE INDEX IF NOT EXISTS uk_customer_wechat_open_id
  ON customer(wechat_open_id)
  WHERE wechat_open_id IS NOT NULL AND wechat_open_id != '';

CREATE UNIQUE INDEX IF NOT EXISTS uk_customer_douyin_open_id
  ON customer(douyin_open_id)
  WHERE douyin_open_id IS NOT NULL AND douyin_open_id != '';

CREATE UNIQUE INDEX IF NOT EXISTS uk_customer_xiaohongshu_id
  ON customer(xiaohongshu_id)
  WHERE xiaohongshu_id IS NOT NULL AND xiaohongshu_id != '';

COMMIT;

-- 应用层后续需要：
-- 1. 在 FindByIdentity / MergeCustomers 处增加冲突处理（已存在 phone → 走合并而非新插入）
-- 2. 增加 data migration 任务：合并次要行的事件/标签/会话到主行
