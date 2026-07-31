-- 028_customer_tags_uuid.sql
-- 修正 customer_tags.id 从 SERIAL(整型) 改为 varchar(36)（UUID/字符串主键），
-- 与 GORM model `model.CustomerTag{ID string gorm:"primaryKey;type:varchar(36)"}` 对齐。
-- 兼容模式：若已是 varchar(36) 则幂等跳过。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'customer_tags' AND column_name = 'id' AND data_type = 'integer'
    ) THEN
        -- 1. 主键序列不再需要，删除以避免 pg_dump 误判
        DROP SEQUENCE IF EXISTS customer_tags_id_seq;
        -- 2. 字段类型从 integer 改为 varchar(36)，先转 text
        ALTER TABLE customer_tags ALTER COLUMN id TYPE VARCHAR(36) USING id::VARCHAR;
        -- 3. 去掉默认（nextval）
        ALTER TABLE customer_tags ALTER COLUMN id DROP DEFAULT;
        -- 4. 字段长度补齐：name(varchar 50) 与 description（新增列）
        ALTER TABLE customer_tags ALTER COLUMN name TYPE VARCHAR(50);
        ALTER TABLE customer_tags ADD COLUMN IF NOT EXISTS category VARCHAR(32);
        ALTER TABLE customer_tags ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'manual';
        ALTER TABLE customer_tags ADD COLUMN IF NOT EXISTS rule TEXT;
    ELSE
        -- 已是 varchar(36)，但补齐可能缺的列（幂等）
        ALTER TABLE customer_tags ADD COLUMN IF NOT EXISTS category VARCHAR(32);
        ALTER TABLE customer_tags ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'manual';
        ALTER TABLE customer_tags ADD COLUMN IF NOT EXISTS rule TEXT;
    END IF;
END$$;
