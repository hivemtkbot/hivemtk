-- ============================================================
-- 036_hivemtk_platform_cs_agent_data.sql
-- HiveMTK 平台客服智能体（seed-hivemtk-product-service，agent 83）专属种子数据
-- 目的：把该智能体的「资产包 / FAQ / SOP / 全渠道绑定」固化为种子数据，
--       避免仅存在于运行库、重装库后丢失（此前这些仅以 DB 直改方式存在）。
-- 依赖：
--   - 031_platform_cs_rag_seed.sql（RAG 知识库 hivemtk-platform-cs）
--   - 033_industry_ai_agents_seed.sql（已创建该智能体并绑定 RAG）
-- 幂等：重跑安全。先清理本智能体专属的 FAQ/SOP/资产包与渠道绑定再重建；
--       通过 agent_code 解析 agent_id，兼容历史 code。
-- ============================================================

DO $$
DECLARE
    v_agent_id BIGINT;
    v_faq_ids  BIGINT[];
    v_sop_ids  BIGINT[];
BEGIN
    -- 解析平台客服智能体（兼容历史 code hivemtk-agent-platform-cs）
    SELECT id INTO v_agent_id
      FROM ai_agents
     WHERE agent_code IN ('seed-hivemtk-product-service', 'hivemtk-agent-platform-cs')
     ORDER BY id
     LIMIT 1;

    IF v_agent_id IS NULL THEN
        RAISE NOTICE 'agent seed-hivemtk-product-service not found, skip 036.';
        RETURN;
    END IF;

    -- 1) 幂等清理本智能体专属的 FAQ / SOP / 资产包
    DELETE FROM faq_entries   WHERE agent_id = v_agent_id;
    DELETE FROM sop_templates WHERE agent_id = v_agent_id;
    DELETE FROM asset_bundles WHERE asset_id = 'hivemtk-platform-cs-bundle';

    -- 2) 资产包：HiveMTK 平台客服接待资产包（行业=platform，全 HiveMTK 专属话术）
    INSERT INTO asset_bundles (
        asset_id, title, description, author, version, scope, status,
        industry, language, tags, messages, examples, supported_languages,
        created_at, updated_at
    ) VALUES (
        'hivemtk-platform-cs-bundle',
        'HiveMTK 平台客服接待资产包',
        'HiveMTK 开源 AI 智能体平台官方客服接待 SOP：覆盖平台介绍、部署、渠道接入、资产市场、智能体配置等意图分流，演示技术可信、简洁、引导式话术。',
        'hivemtk',
        '1.0.0',
        'public',
        'active',
        'platform',
        'zh',
        ARRAY['hivemtk','客服','接待'],
        '[
          {"role":"greeting","content":"您好，我是 HiveMTK 官方客服助手，很高兴为您服务～您可以问我：HiveMTK 是什么、怎么部署、支持哪些渠道、AI 智能体怎么用、资产市场怎么玩。","weight":1.0},
          {"role":"recommend","content":"要不要先看看 HiveMTK 的【开源地址】和【5 分钟部署教程】？我可以直接发给您。","weight":0.9},
          {"role":"discovery","content":"方便说下您主要做哪类生意吗？电商 / 本地生活 / 教育 / 招商？我好给您更对口的方案。","weight":0.9},
          {"role":"channel","content":"HiveMTK 已支持抖音、小红书、微信、企微、Telegram、WhatsApp、钉钉、飞书、网页客服等九端接入，您目前最想接哪类渠道？","weight":0.9},
          {"role":"deploy","content":"部署很简单：git clone 后 docker compose up -d，5 分钟跑起网页客服 + AI 智能体，最小 4 核 8G。","weight":0.9},
          {"role":"agent","content":"我们的 AI 智能体是 ReAct 自主型，内置 41 个工具 + 三级 RAG，能自主接待、营销、售后，不用写代码。","weight":0.9},
          {"role":"security","content":"HiveMTK 100% 私有化部署，数据零出域；LLM 默认走本地推理栈，安全合规。","weight":0.9},
          {"role":"asset","content":"还有「资产市场」可以复用别人的智能体资产包、话术、FAQ、SOP，快速搭出您自己的客服。","weight":0.8}
        ]'::jsonb,
        '[
          {"user":"HiveMTK 是什么？","bot":"HiveMTK 是一个开源、可私有化部署的 AI 营销操作系统，把多端渠道接入、AI 智能体、内容生成、资产市场整合到一起。AGPL-3.0 开源，代码在 GitHub / Gitee。"},
          {"user":"怎么部署？","bot":"推荐 Docker Compose 一键部署：git clone 后 docker compose up -d，拉起后端、前端、PostgreSQL、Redis 与本地推理栈，最小 4 核 8G。"},
          {"user":"支持哪些渠道？","bot":"支持抖音、小红书、微信/企微、Telegram、WhatsApp、钉钉、飞书、网页客服共九端，也能通过桥接接入任意第三方渠道。"}
        ]'::jsonb,
        ARRAY['zh'],
        NOW(),
        NOW()
    );

    -- 3) FAQ 条目（HiveMTK 专属），收集 id 回填 ai_agents.faq_entry_ids
    WITH ins AS (
        INSERT INTO faq_entries (
            question, answer, keywords, category, intent, confidence, enabled, agent_id, created_at, updated_at
        ) VALUES
        (
            'HiveMTK 是什么？',
            'HiveMTK 是一个开源、可私有化部署的 AI 营销操作系统（营销中台 / 营销自动化套件），面向中小企业与商户，把多端私域渠道接入、AI 智能体、内容生成、资产市场整合到同一套系统里，帮助商家用更低成本做获客、转化与复购。项目 AGPL-3.0 开源，代码托管在 GitHub / Gitee。',
            ARRAY['HiveMTK','是什么','介绍','开源'],
            '产品介绍', 'what_is', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            'HiveMTK 是开源的吗？',
            '是的，HiveMTK 基于 AGPL-3.0 协议完全开源，后端 Go、前端 Vue3 源码均托管在 GitHub 与 Gitee，可免费自部署、二次开发。',
            ARRAY['开源','AGPL','GitHub','Gitee','源码'],
            '产品介绍', 'open_source', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            'HiveMTK 怎么部署？',
            '推荐用 Docker Compose 一键部署：git clone 仓库后执行 docker compose up -d 即可拉起 user-server（后端）、user-web（前端）、PostgreSQL、Redis 与本地推理栈（llama.cpp / TEI）。最小环境 4 核 8G，生产建议 8 核 16G 以上并挂载 GPU 可选。详细步骤见项目 README 与部署文档。',
            ARRAY['部署','Docker','安装','安装教程'],
            '部署', 'deploy', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            'HiveMTK 支持哪些渠道接入？',
            'HiveMTK 支持七端私域 + 公域渠道接入：抖音（抖音/抖音网页版）、小红书、微信（微信公众号/微信客服/企业微信）、Telegram、WhatsApp、钉钉、飞书，以及网页客服（H5 / 嵌入式 web_embed）。可通过桥接方式把任意第三方网页/定制渠道接入。',
            ARRAY['渠道','接入','对接'],
            '渠道', 'channels', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '什么是资产市场？',
            '资产市场（Asset Market）是 HiveMTK 的共享能力中心，可发布与订阅「智能体资产包 / 话术包 / FAQ / SOP / 知识库 / 内容模板」等，打通不同商户、不同行业之间的营销能力复用，支持私有与公开两种范围。',
            ARRAY['资产市场','资产包','市场'],
            '资产市场', 'asset_market', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '如何配置 AI 智能体？',
            '进入管理后台 → AI 智能体，可创建/编辑智能体，设置人设（persona）、欢迎语、绑定知识库（RAG 产品）、资产包（话术）、FAQ、SOP 与 LLM 提供商；保存后通过渠道绑定把智能体挂载到具体接待渠道即可生效。',
            ARRAY['智能体','配置','设置'],
            '配置', 'agent_config', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            'HiveMTK 的技术栈是什么？',
            '后端 Go 1.25 + Gin + GORM，向量检索用 pgvector；前端 Vue 3 + Vite；数据层 PostgreSQL 15 + Redis 7；本地推理栈 llama.cpp（LLM）+ TEI（Embedding/Rerank）。整体可纯 Docker 部署，支持 x86 / ARM。',
            ARRAY['技术栈','Go','Vue','架构','pgvector'],
            '技术栈', 'tech_stack', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            'HiveMTK 的 AI 智能体是怎么工作的？',
            'HiveMTK 内置 ReAct 自主智能体：智能体在「思考-行动-观察」循环中自主调用 41 个内置工具（知识库检索、内容生成、订单/用户查询、渠道消息、工单等），结合三级 RAG（BM25 + 向量召回 + bge-reranker 重排）与多轮记忆，完成接待、营销、售后等任务。',
            ARRAY['智能体','ReAct','工具','RAG','原理'],
            '智能体原理', 'agent_how', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '使用 HiveMTK 数据安全吗？数据会出域吗？',
            '100% 私有化部署，数据零出域：所有对话、客户资料、知识库都落在你自己的 PostgreSQL / 向量库里；LLM 推理默认走本地推理栈（llama.cpp + TEI），不依赖任何第三方云端。如需接入云端大模型，可自主配置并承担相应出域范围。',
            ARRAY['数据安全','出域','隐私','私有化'],
            '数据安全', 'data_security', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '日常运维和升级怎么处理？',
            '系统以 Docker Compose 编排，升级即 git pull 后 docker compose up -d --force-recreate；日志统一输出，可接 Loki / ELK；PostgreSQL 建议定时 pg_dump 备份。推理栈与业务服务解耦，可独立重启。',
            ARRAY['运维','升级','备份','日志'],
            '运维', 'ops', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            'HiveMTK 的整体架构是怎样的？',
            '采用分层架构：接入层（七端渠道 + 桥接）→ 编排层（ReAct 智能体 + 对话编排 SmartCSOrchestrator）→ 能力层（RAG 知识库、内容生成、41 内置工具、资产市场）→ 数据层（PostgreSQL + pgvector + Redis）。模块化、可水平扩展。',
            ARRAY['架构','分层','模块'],
            '架构', 'architecture', 0.95, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '常见问题如何排查？',
            '可先看服务日志（docker logs）与系统运行监控；RAG 召回异常优先检查知识库切片与 embedding 状态；渠道不通检查渠道账号 webhook / 令牌配置；LLM 无回复检查提供商连通性与 token 上限。复杂问题可转人工坐席。',
            ARRAY['排障','故障','日志','排查'],
            '排障', 'troubleshooting', 0.95, TRUE, v_agent_id, NOW(), NOW()
        )
        RETURNING id
    )
    SELECT array_agg(id ORDER BY id) INTO v_faq_ids FROM ins;

    -- 4) SOP 模板（HiveMTK 专属），收集 id 回填 ai_agents.sop_template_ids
    WITH ins AS (
        INSERT INTO sop_templates (
            name, intent, stage, template, vars, priority, confidence, enabled, agent_id, created_at, updated_at
        ) VALUES
        (
            '开场接待', 'greeting', 'greeting',
            '您好，我是 HiveMTK 官方客服助手～请问您是想了解【开源/部署/渠道接入/AI 智能体/资产市场】中的哪一块？我可以直接给您讲，也可以安排演示。',
            'topic', 10, 0.9, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '需求挖掘-业务场景', 'need_discovery', 'discovery',
            '为了给您更贴合的方案，方便说下您主要做哪类生意（电商/本地生活/教育/招商/其他）？目前在用哪些渠道获客？最想解决的是获客、转化还是复购？',
            'business,channel,pain_point', 20, 0.9, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '引导部署试用', 'deploy_guide', 'conversion',
            '最快的体验方式是 Docker Compose 一键部署：git clone 后 docker compose up -d，约 5 分钟就能本地跑起网页客服 + AI 智能体。需要的话我可以把部署文档和 GitHub 地址发您。',
            'repo_url', 30, 0.9, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '部署引导（详细步骤）', 'deploy_steps', 'conversion',
            '部署分三步：1) 安装 Docker / Docker Compose；2) git clone 仓库并复制 .env 配置；3) docker compose up -d 拉起全部服务。最小 4 核 8G，生产建议 8 核 16G + 可选 GPU。',
            'min_spec', 30, 0.9, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '渠道接入引导', 'channel_guide', 'conversion',
            '渠道接入一般两步：在管理后台「渠道管理」添加对应渠道账号（抖音/小红书/企微/Telegram 等）并填写令牌；再把 HiveMTK 智能体通过渠道绑定挂载到该渠道即可自动接待。网页客服可直接嵌入站点。',
            'channel', 30, 0.9, TRUE, v_agent_id, NOW(), NOW()
        ),
        (
            '故障排查与转人工', 'troubleshoot_and_handoff', 'handoff',
            '如果系统使用中有异常，可以先看服务日志与运行监控；仍无法解决的，我会为您转接人工坐席，由技术支持同学跟进。请描述您遇到的问题现象。',
            'issue', 40, 0.9, TRUE, v_agent_id, NOW(), NOW()
        )
        RETURNING id
    )
    SELECT array_agg(id ORDER BY id) INTO v_sop_ids FROM ins;

    -- 5) 回填智能体绑定（资产包 + FAQ + SOP）
    UPDATE ai_agents
       SET asset_bundle_id  = 'hivemtk-platform-cs-bundle',
           faq_entry_ids    = v_faq_ids,
           sop_template_ids = v_sop_ids,
           updated_at       = NOW()
     WHERE id = v_agent_id;

    -- 6) 全渠道绑定：确保这 12 个（channel_type, account_id）全部归属本智能体
    --    （先删除这些组合既有绑定，再统一插入，保证「所有渠道都指向 HiveMTK 智能体」）
    DELETE FROM channel_agent_bindings
     WHERE (channel_type, account_id) IN (
        ('dingtalk','seed-dingtalk-001'),
        ('douyin','seed-dy-001'),
        ('douyin_web','test-node-1'),
        ('feishu','seed-feishu-001'),
        ('telegram','default'),
        ('telegram','seed-tg-001'),
        ('web','default'),
        ('web_embed','default'),
        ('wecom','seed-wecom-001'),
        ('wecom','seed-wecom-002'),
        ('whatsapp','seed-wa-001'),
        ('xiaohongshu','seed-xhs-001')
    );

    INSERT INTO channel_agent_bindings (channel_type, account_id, agent_id, is_primary, enabled, created_at, updated_at)
    VALUES
        ('dingtalk','seed-dingtalk-001', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('douyin','seed-dy-001', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('douyin_web','test-node-1', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('feishu','seed-feishu-001', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('telegram','default', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('telegram','seed-tg-001', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('web','default', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('web_embed','default', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('wecom','seed-wecom-001', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('wecom','seed-wecom-002', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('whatsapp','seed-wa-001', v_agent_id, TRUE, TRUE, NOW(), NOW()),
        ('xiaohongshu','seed-xhs-001', v_agent_id, TRUE, TRUE, NOW(), NOW());
END $$;

-- ============================================================
-- 验证：确认平台客服智能体已绑定资产包 / FAQ / SOP / 全渠道
-- ============================================================
SELECT aa.agent_code,
       aa.asset_bundle_id,
       array_length(aa.faq_entry_ids, 1)   AS n_faq,
       array_length(aa.sop_template_ids, 1) AS n_sop,
       (SELECT count(*) FROM channel_agent_bindings cab WHERE cab.agent_id = aa.id) AS n_channels
  FROM ai_agents aa
 WHERE aa.agent_code = 'seed-hivemtk-product-service';

SELECT channel_type, account_id, is_primary, enabled
  FROM channel_agent_bindings
 WHERE agent_id = (SELECT id FROM ai_agents WHERE agent_code = 'seed-hivemtk-product-service')
 ORDER BY channel_type, account_id;
