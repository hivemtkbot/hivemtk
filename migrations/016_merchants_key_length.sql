-- ============================================================
-- Migration 016: 扩大 merchants.merchant_key 长度
-- 背景：generateLicenseKey() 生成的授权码格式为
--       XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX（32 hex + 3 dashes = 35 字符）
--       原列定义为 VARCHAR(32)，在 dash 分隔格式下报 22001 错误
-- 方案：调整为 VARCHAR(64)，与 licenses.license_key 一致
-- 影响：仅扩展列长度，无破坏性变更；既有数据不受影响
-- ============================================================

ALTER TABLE merchants ALTER COLUMN merchant_key TYPE VARCHAR(64);
