-- =============================================================================
-- Migration 047: 高频 enum 改 PostgreSQL ENUM（OPT-DB-08 修复版）
-- 2026-08-16 修订
--
-- 背景：项目中有大量 VARCHAR 字段表达"枚举语义"
--       （platform/intent_type/message_status 等），字符串校验完全靠应用层，
--       容易写脏数据 + 索引利用率低。
--
-- 重要：本迁移所有 ENUM 值必须与 Go 代码常量保持一致
--   - 平台渠道: hivemtk/user-server/internal/model/ai_agent.go (ChannelTypeXxx)
--   - 消息状态: hivemtk/user-server/internal/model/unified_message.go (MessageStatusXxx)
--   - 嵌入状态: hivemtk/user-server/internal/model/kb_workspace.go (EmbedStatusXxx)
--   - 文档来源: hivemtk/user-server/internal/model/kb_workspace.go (SourceTypeXxx)
--   - 意图主类: hivemtk/user-server/internal/service/intent_recognition_fine.go (IntentMajorXxx)
--   - 意图子类: hivemtk/user-server/internal/service/intent_recognition_fine.go (IntentMinorXxx)
--
-- 兼容性：
--   - PG 11+ 通用
--   - 转换失败的字符串将自动转 ENUM 失败 → 提前用 bad_count 探测
--   - 全部 IF NOT EXISTS / DO $$ 包裹（幂等）
-- =============================================================================

BEGIN;

-- -----------------------------------------------------------------------------
-- 1) 创建枚举类型（值与 Go ChannelTypeXxx 完全一致）
-- -----------------------------------------------------------------------------
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'platform_type_enum') THEN
    CREATE TYPE platform_type_enum AS ENUM (
      'telegram', 'wecom', 'feishu', 'whatsapp', 'dingtalk',
      'douyin', 'xiaohongshu', 'kuaishou', 'xianyu', 'tiktok',
      'web', 'web_embed'
    );
    RAISE NOTICE 'Created enum platform_type_enum (12 values)';
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'intent_major_enum') THEN
    CREATE TYPE intent_major_enum AS ENUM (
      'consult', 'price_inquiry', 'objection', 'after_sale',
      'complaint', 'churn', 'intent_buy', 'ask_product'
    );
    RAISE NOTICE 'Created enum intent_major_enum (8 values, matches Go IntentMajorXxx)';
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'message_status_enum') THEN
    CREATE TYPE message_status_enum AS ENUM (
      'pending', 'processing', 'replied', 'failed', 'ignored'
    );
    RAISE NOTICE 'Created enum message_status_enum (5 values, matches Go MessageStatusXxx)';
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'embed_status_enum') THEN
    CREATE TYPE embed_status_enum AS ENUM (
      'pending', 'processing', 'indexed', 'failed'
    );
    RAISE NOTICE 'Created enum embed_status_enum (4 values, matches Go EmbedStatusXxx)';
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'source_type_enum') THEN
    CREATE TYPE source_type_enum AS ENUM (
      'upload', 'text', 'url', 'batch', 'openapi'
    );
    RAISE NOTICE 'Created enum source_type_enum (5 values, matches Go SourceTypeXxx)';
  END IF;
END $$;

-- -----------------------------------------------------------------------------
-- 2) message_hub.platform → platform_type_enum
--    注：message_hub 表的 platform 列历史上可能存了 'xhs'(xiaohongshu 简写)、
--       'wechat'(公众号) 等与 Go 常量不一致的值，转换前需要先归一化
-- -----------------------------------------------------------------------------
DO $$
DECLARE
  bad_count INTEGER;
  xhs_count INTEGER;
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'message_hub') THEN
    -- 归一化历史遗留值
    UPDATE message_hub SET platform = 'xiaohongshu' WHERE platform = 'xhs';
    UPDATE message_hub SET platform = 'wecom' WHERE platform = 'wechat';
    UPDATE message_hub SET platform = 'system' WHERE platform NOT IN (
      'telegram', 'wecom', 'feishu', 'whatsapp', 'dingtalk',
      'douyin', 'xiaohongshu', 'kuaishou', 'xianyu', 'tiktok',
      'web', 'web_embed', 'system'
    );

    SELECT COUNT(*) INTO bad_count
    FROM message_hub
    WHERE platform IS NOT NULL
      AND platform NOT IN (
        'telegram', 'wecom', 'feishu', 'whatsapp', 'dingtalk',
        'douyin', 'xiaohongshu', 'kuaishou', 'xianyu', 'tiktok',
        'web', 'web_embed', 'system'
      );
    IF bad_count > 0 THEN
      RAISE WARNING 'message_hub.platform still has % unknown values after normalization, skipping ENUM conversion', bad_count;
    ELSE
      ALTER TABLE message_hub
        ALTER COLUMN platform TYPE platform_type_enum USING platform::platform_type_enum;
      RAISE NOTICE 'Converted message_hub.platform to platform_type_enum';
    END IF;
  END IF;
END $$;

-- -----------------------------------------------------------------------------
-- 3) intent_records.intent_major → intent_major_enum
--    注意：原 schema 中字段名是 intent_type,我们的 Go 常量是 IntentMajor
--    因此这里使用 intent_major 作为新列名（不破坏兼容）
-- -----------------------------------------------------------------------------
DO $$
DECLARE
  bad_count INTEGER;
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'intent_records') THEN
    -- 添加 intent_major 列（与 intent_type 并存）
    IF NOT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_name = 'intent_records' AND column_name = 'intent_major'
    ) THEN
      ALTER TABLE intent_records ADD COLUMN intent_major VARCHAR(50);
    END IF;

    -- 从 intent_type 列回填 intent_major（仅针对 Go 定义的 8 个值）
    UPDATE intent_records SET intent_major = intent_type
    WHERE intent_type IN (
      'consult', 'price_inquiry', 'objection', 'after_sale',
      'complaint', 'churn', 'intent_buy', 'ask_product'
    ) AND (intent_major IS NULL OR intent_major = '');

    -- 检查归一化后是否有脏值
    SELECT COUNT(*) INTO bad_count FROM intent_records
    WHERE intent_major IS NOT NULL AND intent_major != ''
      AND intent_major NOT IN (
        'consult', 'price_inquiry', 'objection', 'after_sale',
        'complaint', 'churn', 'intent_buy', 'ask_product'
      );

    IF bad_count > 0 THEN
      RAISE WARNING 'intent_records.intent_major has % unknown values, skipping ENUM conversion', bad_count;
    ELSE
      ALTER TABLE intent_records
        ALTER COLUMN intent_major TYPE intent_major_enum USING intent_major::intent_major_enum;
      RAISE NOTICE 'Converted intent_records.intent_major to intent_major_enum';
    END IF;
  END IF;
END $$;

-- -----------------------------------------------------------------------------
-- 4) unified_messages.status → message_status_enum
-- -----------------------------------------------------------------------------
DO $$
DECLARE
  bad_count INTEGER;
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'unified_messages') THEN
    SELECT COUNT(*) INTO bad_count FROM unified_messages
    WHERE status IS NOT NULL
      AND status NOT IN ('pending', 'processing', 'replied', 'failed', 'ignored');

    IF bad_count > 0 THEN
      RAISE WARNING 'unified_messages.status has % unknown values, skipping', bad_count;
    ELSE
      ALTER TABLE unified_messages
        ALTER COLUMN status TYPE message_status_enum USING status::message_status_enum;
      RAISE NOTICE 'Converted unified_messages.status to message_status_enum';
    END IF;
  END IF;
END $$;

-- -----------------------------------------------------------------------------
-- 5) knowledge_documents.embed_status → embed_status_enum
-- -----------------------------------------------------------------------------
DO $$
DECLARE
  bad_count INTEGER;
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'knowledge_documents') THEN
    -- 归一化历史 'indexed' 值
    UPDATE knowledge_documents SET embed_status = 'indexed' WHERE embed_status = 'indexed';
    -- 'completed' / 'done' 归一化
    UPDATE knowledge_documents SET embed_status = 'indexed'
    WHERE embed_status IN ('completed', 'done', 'success');

    SELECT COUNT(*) INTO bad_count FROM knowledge_documents
    WHERE embed_status IS NOT NULL
      AND embed_status NOT IN ('pending', 'processing', 'indexed', 'failed');

    IF bad_count > 0 THEN
      RAISE WARNING 'knowledge_documents.embed_status has % unknown values, skipping', bad_count;
    ELSE
      ALTER TABLE knowledge_documents
        ALTER COLUMN embed_status TYPE embed_status_enum USING embed_status::embed_status_enum;
      RAISE NOTICE 'Converted knowledge_documents.embed_status to embed_status_enum';
    END IF;
  END IF;
END $$;

-- -----------------------------------------------------------------------------
-- 6) knowledge_documents.source_type → source_type_enum
-- -----------------------------------------------------------------------------
DO $$
DECLARE
  bad_count INTEGER;
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'knowledge_documents') THEN
    -- 归一化 'import' / 'crawl' 等历史值
    UPDATE knowledge_documents SET source_type = 'upload'
    WHERE source_type IN ('import', 'crawl', 'manual', 'sync', 'imported');

    SELECT COUNT(*) INTO bad_count FROM knowledge_documents
    WHERE source_type IS NOT NULL
      AND source_type NOT IN ('upload', 'text', 'url', 'batch', 'openapi');

    IF bad_count > 0 THEN
      RAISE WARNING 'knowledge_documents.source_type has % unknown values, skipping', bad_count;
    ELSE
      ALTER TABLE knowledge_documents
        ALTER COLUMN source_type TYPE source_type_enum USING source_type::source_type_enum;
      RAISE NOTICE 'Converted knowledge_documents.source_type to source_type_enum';
    END IF;
  END IF;
END $$;

COMMIT;

-- =============================================================================
-- 一致性校验（人工执行，CI 不跑）
-- 1) 对比 Go 常量：
--   - ChannelTypeXxx (ai_agent.go)        ↔ platform_type_enum
--   - IntentMajorXxx (intent_recognition_fine.go) ↔ intent_major_enum
--   - MessageStatusXxx (unified_message.go) ↔ message_status_enum
--   - EmbedStatusXxx (kb_workspace.go)    ↔ embed_status_enum
--   - SourceTypeXxx (kb_workspace.go)     ↔ source_type_enum
--
-- 2) 推荐加一个 CI 步骤（scripts/check-enum-consistency.sh）：
--    解析 Go 源码中的 `= "xxx"` 常量，对比 PG ENUM 列表，差异即 fail
-- =============================================================================

-- =============================================================================
-- 回滚（如需）：
--   ALTER TABLE message_hub ALTER COLUMN platform TYPE VARCHAR(30);
--   ALTER TABLE intent_records ALTER COLUMN intent_major TYPE VARCHAR(50);
--   ALTER TABLE unified_messages ALTER COLUMN status TYPE VARCHAR(20);
--   ALTER TABLE knowledge_documents ALTER COLUMN embed_status TYPE VARCHAR(16);
--   ALTER TABLE knowledge_documents ALTER COLUMN source_type TYPE VARCHAR(16);
--   DROP TYPE IF EXISTS platform_type_enum;
--   DROP TYPE IF EXISTS intent_major_enum;
--   DROP TYPE IF EXISTS message_status_enum;
--   DROP TYPE IF EXISTS embed_status_enum;
--   DROP TYPE IF EXISTS source_type_enum;
-- =============================================================================
