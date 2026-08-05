-- ============================================================
-- 037_bridge_channel_unify_v2.sql
-- 2026-08-05 渠道编码统一 v2：把 *_web / xhs 全部归一化为全名
-- 与 Go migration `BridgeChannelUnifyV2Migration` (v3.18.0) 完全等价。
-- 单一源：marketing/internal/model/message_event.go ChannelXHS/Douyin/Kuaishou/Xianyu/TikTok
-- ============================================================

-- 1) message_hub.platform（仅 bridge 来源）
DO $$
DECLARE
    r INT;
    old_name TEXT;
    new_name TEXT;
BEGIN
    FOREACH old_name IN ARRAY ARRAY['xhs_web', 'douyin_web', 'kuaishou_web', 'xianyu_web', 'tiktok_web', 'xhs']
    LOOP
        new_name := CASE old_name
            WHEN 'xhs_web'      THEN 'xiaohongshu'
            WHEN 'douyin_web'   THEN 'douyin'
            WHEN 'kuaishou_web' THEN 'kuaishou'
            WHEN 'xianyu_web'   THEN 'xianyu'
            WHEN 'tiktok_web'   THEN 'tiktok'
            WHEN 'xhs'          THEN 'xiaohongshu'
        END;
        UPDATE message_hub
           SET platform = new_name
         WHERE platform = old_name
           AND extra::text LIKE '%"bridge":true%';
        GET DIAGNOSTICS r = ROW_COUNT;
        IF r > 0 THEN
            RAISE NOTICE '[037] message_hub(bridge): % rows % -> %', r, old_name, new_name;
        END IF;
    END LOOP;
END$$;

-- 2) customer_sessions.platform
DO $$
DECLARE
    r INT;
    old_name TEXT;
    new_name TEXT;
BEGIN
    FOREACH old_name IN ARRAY ARRAY['xhs_web', 'douyin_web', 'kuaishou_web', 'xianyu_web', 'tiktok_web', 'xhs']
    LOOP
        new_name := CASE old_name
            WHEN 'xhs_web'      THEN 'xiaohongshu'
            WHEN 'douyin_web'   THEN 'douyin'
            WHEN 'kuaishou_web' THEN 'kuaishou'
            WHEN 'xianyu_web'   THEN 'xianyu'
            WHEN 'tiktok_web'   THEN 'tiktok'
            WHEN 'xhs'          THEN 'xiaohongshu'
        END;
        UPDATE customer_sessions SET platform = new_name WHERE platform = old_name;
        GET DIAGNOSTICS r = ROW_COUNT;
        IF r > 0 THEN RAISE NOTICE '[037] customer_sessions: % rows % -> %', r, old_name, new_name; END IF;
    END LOOP;
END$$;

-- 3) inbox_conversations.platform
DO $$
DECLARE
    r INT;
    old_name TEXT;
    new_name TEXT;
BEGIN
    FOREACH old_name IN ARRAY ARRAY['xhs_web', 'douyin_web', 'kuaishou_web', 'xianyu_web', 'tiktok_web', 'xhs']
    LOOP
        new_name := CASE old_name
            WHEN 'xhs_web'      THEN 'xiaohongshu'
            WHEN 'douyin_web'   THEN 'douyin'
            WHEN 'kuaishou_web' THEN 'kuaishou'
            WHEN 'xianyu_web'   THEN 'xianyu'
            WHEN 'tiktok_web'   THEN 'tiktok'
            WHEN 'xhs'          THEN 'xiaohongshu'
        END;
        UPDATE inbox_conversations SET platform = new_name WHERE platform = old_name;
        GET DIAGNOSTICS r = ROW_COUNT;
        IF r > 0 THEN RAISE NOTICE '[037] inbox_conversations: % rows % -> %', r, old_name, new_name; END IF;
    END LOOP;
END$$;

-- 4) bridge_accounts.channel
DO $$
DECLARE
    r INT;
    old_name TEXT;
    new_name TEXT;
BEGIN
    FOREACH old_name IN ARRAY ARRAY['xhs_web', 'douyin_web', 'kuaishou_web', 'xianyu_web', 'tiktok_web', 'xhs']
    LOOP
        new_name := CASE old_name
            WHEN 'xhs_web'      THEN 'xiaohongshu'
            WHEN 'douyin_web'   THEN 'douyin'
            WHEN 'kuaishou_web' THEN 'kuaishou'
            WHEN 'xianyu_web'   THEN 'xianyu'
            WHEN 'tiktok_web'   THEN 'tiktok'
            WHEN 'xhs'          THEN 'xiaohongshu'
        END;
        UPDATE bridge_accounts SET channel = new_name WHERE channel = old_name;
        GET DIAGNOSTICS r = ROW_COUNT;
        IF r > 0 THEN RAISE NOTICE '[037] bridge_accounts: % rows % -> %', r, old_name, new_name; END IF;
    END LOOP;
END$$;

-- 5) channel_agent_bindings.channel_type
DO $$
DECLARE
    r INT;
    old_name TEXT;
    new_name TEXT;
BEGIN
    FOREACH old_name IN ARRAY ARRAY['xhs_web', 'douyin_web', 'kuaishou_web', 'xianyu_web', 'tiktok_web', 'xhs']
    LOOP
        new_name := CASE old_name
            WHEN 'xhs_web'      THEN 'xiaohongshu'
            WHEN 'douyin_web'   THEN 'douyin'
            WHEN 'kuaishou_web' THEN 'kuaishou'
            WHEN 'xianyu_web'   THEN 'xianyu'
            WHEN 'tiktok_web'   THEN 'tiktok'
            WHEN 'xhs'          THEN 'xiaohongshu'
        END;
        UPDATE channel_agent_bindings SET channel_type = new_name WHERE channel_type = old_name;
        GET DIAGNOSTICS r = ROW_COUNT;
        IF r > 0 THEN RAISE NOTICE '[037] channel_agent_bindings: % rows % -> %', r, old_name, new_name; END IF;
    END LOOP;
END$$;

-- 6) ai_suggestions.session_id 前缀归一化（session_id 形如 "xhs_web:xxx:yyy"）
--    session_id 内的 platform 前缀必须与 message_hub.platform 一致，
--    否则 findOrCreateSession 按 session_id 索引会查不到对应会话。
DO $$
DECLARE
    r INT;
    old_name TEXT;
    new_name TEXT;
BEGIN
    FOREACH old_name IN ARRAY ARRAY['xhs_web', 'douyin_web', 'kuaishou_web', 'xianyu_web', 'tiktok_web', 'xhs']
    LOOP
        new_name := CASE old_name
            WHEN 'xhs_web'      THEN 'xiaohongshu'
            WHEN 'douyin_web'   THEN 'douyin'
            WHEN 'kuaishou_web' THEN 'kuaishou'
            WHEN 'xianyu_web'   THEN 'xianyu'
            WHEN 'tiktok_web'   THEN 'tiktok'
            WHEN 'xhs'          THEN 'xiaohongshu'
        END;
        -- 仅替换 session_id 的「前缀 + :」部分（保留后续 :accountID:conversationID 不变）
        EXECUTE format(
            'UPDATE ai_suggestions SET session_id = %L || SUBSTRING(session_id FROM %L) WHERE session_id LIKE %L',
            new_name,
            LENGTH(old_name) + 1,
            old_name || ':%'
        );
        GET DIAGNOSTICS r = ROW_COUNT;
        IF r > 0 THEN RAISE NOTICE '[037] ai_suggestions.session_id: % rows %: -> %:', r, old_name, new_name; END IF;
    END LOOP;
END$$;
