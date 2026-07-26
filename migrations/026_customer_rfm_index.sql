-- ============================================================================
-- Migration 026: 客户中心 RFM 数据源索引修复 (F-P0-08)
-- 目的:
--   1. order 表新增 account_id 索引：加速 OrderRepository.GetByCustomerID
--      （通过 customers.phone 子查询匹配 order.account_id）
--   2. order 表新增 tg_id 索引（如缺失）：兼容历史 GetByTgID 路径
--   3. customers 表 phone 字段已有索引（model 中声明 index），此处幂等补建
--   4. customer_rfms 表新增 segment + composite_score 复合索引：分层查询提速
-- 关联功能项: F-P0-08 RFM 数据源修复
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. order 表索引
-- ----------------------------------------------------------------------------
-- account_id 索引：手机号客户主路径（GetByCustomerID 通过 phone 子查询匹配）
CREATE INDEX IF NOT EXISTS idx_order_account_id ON "order" (account_id);

-- tg_id 索引：兼容历史 GetByTgID 路径（如已存在则跳过）
CREATE INDEX IF NOT EXISTS idx_order_tg_id ON "order" (tg_id);

-- create_time 索引：RFM recency 计算依赖时间排序
CREATE INDEX IF NOT EXISTS idx_order_create_time ON "order" (create_time);

-- ----------------------------------------------------------------------------
-- 2. customers 表索引（幂等补建，model 中已声明 gorm:"index"）
-- ----------------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_customers_phone ON customers (phone);
CREATE INDEX IF NOT EXISTS idx_customers_email ON customers (email);

-- ----------------------------------------------------------------------------
-- 3. customer_rfms 表索引（分层查询 / RFM 标签更新加速）
-- ----------------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_customer_rfms_segment ON customer_rfms (segment);
CREATE INDEX IF NOT EXISTS idx_customer_rfms_composite_score ON customer_rfms (composite_score DESC);
CREATE INDEX IF NOT EXISTS idx_customer_rfms_customer_id ON customer_rfms (customer_id);

-- ----------------------------------------------------------------------------
-- 4. 注释
-- ----------------------------------------------------------------------------
COMMENT ON INDEX idx_order_account_id IS 'F-P0-08: 加速按 account_id（手机号/UUID）查询订单，用于 RFM 计算';
COMMENT ON INDEX idx_customer_rfms_segment IS 'F-P0-08: 加速 RFM 分层分页查询';
