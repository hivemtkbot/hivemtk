-- ============================================================
-- 013_version_offline_fields.sql
-- 给 versions 表追加离线包相关字段（兼容已有库升级）
-- ============================================================

ALTER TABLE versions ADD COLUMN IF NOT EXISTS offline_package_url TEXT;
ALTER TABLE versions ADD COLUMN IF NOT EXISTS docker_offline_url TEXT;
ALTER TABLE versions ADD COLUMN IF NOT EXISTS offline_package_sha256 VARCHAR(64);
ALTER TABLE versions ADD COLUMN IF NOT EXISTS offline_package_size BIGINT;
