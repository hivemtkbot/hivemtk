-- =============================================================================
-- Migration 039: 补全 FK 约束（OPT-DB-01）
-- 2026-08-16
--
-- 背景：原 GORM `DisableForeignKeyConstraintWhenMigrating=true` 关闭了 FK 自动创建。
--       审计发现 99% 表无 FK 约束，导致孤儿记录风险。
--       本迁移为关键引用关系补 FK，应用层补 ON DELETE 策略。
--
-- 原则：
--   - 仅对 customer_id / merchant_id / agent_id / sop_id / knowledge_base_id 等高风险字段建 FK
--   - 全部使用 `NOT VALID` + 后续 `VALIDATE CONSTRAINT` 模式，避免长时间锁表
--   - 应用层保留手动级联逻辑（短期兼容）
-- =============================================================================

BEGIN;

-- 1. 线索线索 → 客户（clue.customer_id → customer.id）
-- 业务：线索转化后必须关联到有效客户
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_clue_customer_id'
      AND table_name = 'clue'
  ) THEN
    ALTER TABLE clue
      ADD CONSTRAINT fk_clue_customer_id
      FOREIGN KEY (customer_id)
      REFERENCES customer(id)
      ON DELETE SET NULL
      ON UPDATE CASCADE
      NOT VALID;
    RAISE NOTICE 'Created fk_clue_customer_id';
  END IF;
END $$;

-- 2. SOP 执行 → SOP 智能体（sop_execution.sop_agent_id → sop_agent.id）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_sop_execution_sop_agent'
      AND table_name = 'sop_execution'
  ) THEN
    ALTER TABLE sop_execution
      ADD CONSTRAINT fk_sop_execution_sop_agent
      FOREIGN KEY (sop_agent_id)
      REFERENCES sop_agent(id)
      ON DELETE CASCADE
      ON UPDATE CASCADE
      NOT VALID;
    RAISE NOTICE 'Created fk_sop_execution_sop_agent';
  END IF;
END $$;

-- 3. 短链访问日志 → 短链（short_link_access.short_link_id → short_link.id）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_short_link_access_short_link'
      AND table_name = 'short_link_access'
  ) THEN
    ALTER TABLE short_link_access
      ADD CONSTRAINT fk_short_link_access_short_link
      FOREIGN KEY (short_link_id)
      REFERENCES short_link(id)
      ON DELETE CASCADE
      ON UPDATE CASCADE
      NOT VALID;
    RAISE NOTICE 'Created fk_short_link_access_short_link';
  END IF;
END $$;

-- 4. A/B 实验转化事件 → 实验（ab_conversion_event.experiment_id → ab_experiment.id）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_ab_conv_event_experiment'
      AND table_name = 'ab_conversion_event'
  ) THEN
    ALTER TABLE ab_conversion_event
      ADD CONSTRAINT fk_ab_conv_event_experiment
      FOREIGN KEY (experiment_id)
      REFERENCES ab_experiment(id)
      ON DELETE CASCADE
      ON UPDATE CASCADE
      NOT VALID;
    RAISE NOTICE 'Created fk_ab_conv_event_experiment';
  END IF;
END $$;

-- 5. A/B 实验转化事件 → 变体（ab_conversion_event.variant_id → ab_variant.id）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_ab_conv_event_variant'
      AND table_name = 'ab_conversion_event'
  ) THEN
    ALTER TABLE ab_conversion_event
      ADD CONSTRAINT fk_ab_conv_event_variant
      FOREIGN KEY (variant_id)
      REFERENCES ab_variant(id)
      ON DELETE CASCADE
      ON UPDATE CASCADE
      NOT VALID;
    RAISE NOTICE 'Created fk_ab_conv_event_variant';
  END IF;
END $$;

-- 6. 知识库文档 → 知识库（knowledge_document.knowledge_base_id → knowledge_base.id）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_knowledge_doc_kb'
      AND table_name = 'knowledge_document'
  ) THEN
    ALTER TABLE knowledge_document
      ADD CONSTRAINT fk_knowledge_doc_kb
      FOREIGN KEY (knowledge_base_id)
      REFERENCES knowledge_base(id)
      ON DELETE CASCADE
      ON UPDATE CASCADE
      NOT VALID;
    RAISE NOTICE 'Created fk_knowledge_doc_kb';
  END IF;
END $$;

-- 7. 对话记忆 → 会话（dialogue_memory.session_id → customer_session.session_id）
-- 注：customer_session.session_id 是 VARCHAR，dialogue_memory.session_id 也是 VARCHAR
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_dialogue_memory_session'
      AND table_name = 'dialogue_memory'
  ) THEN
    ALTER TABLE dialogue_memory
      ADD CONSTRAINT fk_dialogue_memory_session
      FOREIGN KEY (session_id)
      REFERENCES customer_session(session_id)
      ON DELETE CASCADE
      ON UPDATE CASCADE
      NOT VALID;
    RAISE NOTICE 'Created fk_dialogue_memory_session';
  END IF;
END $$;

-- 8. 销冠意图打分 → 客户（sales_intent_score.customer_id → customer.id）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_sales_intent_customer'
      AND table_name = 'sales_intent_score'
  ) THEN
    ALTER TABLE sales_intent_score
      ADD CONSTRAINT fk_sales_intent_customer
      FOREIGN KEY (customer_id)
      REFERENCES customer(id)
      ON DELETE CASCADE
      ON UPDATE CASCADE
      NOT VALID;
    RAISE NOTICE 'Created fk_sales_intent_customer';
  END IF;
END $$;

-- 9. inbox 对话 → inbox 分配（inbox_conversation.assignment_id → inbox_assignment.id）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_inbox_conv_assignment'
      AND table_name = 'inbox_conversation'
  ) THEN
    ALTER TABLE inbox_conversation
      ADD CONSTRAINT fk_inbox_conv_assignment
      FOREIGN KEY (assignment_id)
      REFERENCES inbox_assignment(id)
      ON DELETE SET NULL
      ON UPDATE CASCADE
      NOT VALID;
    RAISE NOTICE 'Created fk_inbox_conv_assignment';
  END IF;
END $$;

COMMIT;

-- 注释：NOT VALID 模式让新约束对存量数据不立即生效（避免长时间锁表）
-- 应用层在 OPT-DB-01 二期会逐表执行 VALIDATE CONSTRAINT 验证存量数据一致性
