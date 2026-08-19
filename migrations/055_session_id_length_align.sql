-- 055_session_id_length_align.sql
-- 扩展 customer_sessions 及关联表 session_id varchar(50) → varchar(120)
-- 修复小红书等长渠道名 stable_id 超长导致 SQLSTATE 22001
-- stable_id 格式: sess_{platform}_{account_id}_{one_id}
-- 小红书示例: sess_xiaohongshu_69c730300000000034018cb2_69c62722000000003203b0fe (60 字符)
-- 关联版本: v3.22.4

ALTER TABLE customer_sessions     ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE session_messages      ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE feedback_events       ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE feedback_signals      ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE champion_dialogues    ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE sop_node_transitions  ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE ai_suggestions        ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE intent_logs           ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE user_blacklist        ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE layer_decision_logs   ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE sla_violations        ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE intent_records        ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE dialogue_memories     ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE sop_executions        ALTER COLUMN session_id TYPE VARCHAR(120);
ALTER TABLE ai_sales_logs         ALTER COLUMN session_id TYPE VARCHAR(120);