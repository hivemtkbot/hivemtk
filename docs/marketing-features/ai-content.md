# AI 内容创作 (AI Content)

> **所属模块**: content-creation
> **功能 slug**: `ai-content`
> **文档定位**: AI 驱动的内容生成工作台,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | AI 内容创作 |
| 功能名称(英文) | AI Content Generation |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | content-creation |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端创作工作台
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 多模态内容生成(图片/视频脚本)

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户需要持续生产营销文案(朋友圈、小红书、抖音、邮件、短信),但人工创作效率低,质量参差。AI 辅助能大幅提升产出与质量。

### 2.3 关键算法或模型

- **Prompt 模板**: `Role + Task + Context + Constraints + Examples + Output`
- **风格库**: 朋友圈轻松 / 抖音活泼 / 小红书种草 / 邮件正式 / 短信简洁
- **变量替换**: `{{product_name}}`、`{{price}}`、`{{customer_name}}`
- **批量生成**: 多产品 × 多风格 → 矩阵化产出
- **评分模型**: 用户对生成结果的点赞/采纳,反馈到模型微调

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | platform | string | 是 | 目标平台 |
| 输入 | content_type | string | 是 | 朋友圈/小红书/邮件 |
| 输入 | product_info | object | 是 | 产品信息 |
| 输入 | target_audience | string | 否 | 目标人群 |
| 输入 | style | string | 否 | 风格 |
| 输入 | keywords | array | 否 | 关键词 |
| 输入 | template_id | int64 | 否 | 套用模板 |
| 输入 | variables | object | 否 | 自定义变量 |
| 输出 | content_id | int64 | 是 | 内容 ID |
| 输出 | generated_text | string | 是 | 生成内容 |
| 输出 | tokens_used | int | 是 | Token 消耗 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/ai/generate | 内容生成 |
| GET | /api/ai/history | 历史记录 |
| POST | /api/ai/save | 保存内容 |
| POST | /api/ai/favorite | 收藏 |
| POST | /api/ai/rate | 评分 |
| DELETE | /api/ai/:id | 删除 |
| GET | /api/ai/templates | 模板列表 |
| POST | /api/ai/templates | 创建模板 |
| PUT | /api/ai/templates/:id | 更新模板 |
| DELETE | /api/ai/templates/:id | 删除模板 |

### 3.3 安全与合规

- 内容合规检测(违规词、政治敏感、虚假宣传)
- LLM API Key 加密存储
- 用户提示词审计(防 Prompt 注入)
- 频率限制(单商户每分钟 10 次)
- 内容溯源(记录生成参数与时间)

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 单次生成 | < 8s (P95) |
| 批量生成(10 篇) | < 30s |
| Token 用量统计 | 实时 |
| 并发生成 | ≥ 50 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/ai | |
| Service | internal/service/ai | 内容生成 + 模板 |
| Engine | internal/service/ai/generator | LLM 调用引擎 |
| Repository | internal/repository/ai | |
| Model | internal/model/ai | |
| Infra | internal/infra/llm | LLM 客户端 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| LLM 网关 | 统一调用 OpenAI/通义/DeepSeek |
| 内容审核 | 违规词检测 |
| 话术库 | 模板市场 |
| 素材管理 | 图片素材引用 |

### 4.3 数据流向

```text
[用户输入: 平台/产品/风格]
        ↓
[Prompt 模板渲染 + 变量替换]
        ↓
[LLM 网关: 多模型路由]
        ↓
[合规检测: 违规词过滤]
        ↓
[结果返回 + 评分反馈]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"内容生产 → AI 内容"
2. 选择目标平台与内容类型
3. 填写产品信息/选择产品库
4. 选择风格(可选)
5. 选择模板(可选)
6. 点击"生成"
7. 查看生成结果(1-N 个候选)
8. 编辑/复制/收藏/评分
9. 保存到内容库

### 5.2 系统处理流程

1. 接收生成请求
2. 加载模板(如有)
3. 渲染 Prompt
4. 调用 LLM(多模型路由按性价比选择)
5. 解析响应
6. 合规检测
7. 写入历史记录
8. 返回结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| LLM 调用超时 | 500100 | 重试 1 次,失败则降级到备用模型 |
| 内容违规 | 500101 | 拒绝返回,提示修改 |
| 频率超限 | 429001 | 返回限流提示 |
| Prompt 注入 | 400100 | 拒绝执行 |

### 5.4 状态机

```text
[生成中] → [成功] → [已保存/已收藏/已删除]
       ↓
     [失败] → [重试] → [成功/失败]
```

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `ai_content_history` | 生成历史 |
| `ai_content_templates` | 创作模板 |
| `ai_content_ratings` | 评分记录 |

```sql
CREATE TABLE ai_content_history (
  id BIGINT PRIMARY KEY,
  
  user_id BIGINT NOT NULL,
  platform VARCHAR(32) NOT NULL,
  content_type VARCHAR(32) NOT NULL,
  prompt TEXT NOT NULL,
  input_params JSONB,
  generated_text TEXT NOT NULL,
  model VARCHAR(64),  -- gpt-4/qwen/deepseek
  tokens_used INT,
  duration_ms INT,
  is_favorite BOOLEAN DEFAULT false,
  rating INT,  -- 1-5
  status VARCHAR(16) DEFAULT 'success',  -- success/failed/timeout
  error_message TEXT,
  created_at TIMESTAMP NOT NULL,
  INDEX idx_data, user_id, created_at)
);

CREATE TABLE ai_content_templates (
  id BIGINT PRIMARY KEY,
  
  name VARCHAR(128) NOT NULL,
  platform VARCHAR(32),
  content_type VARCHAR(32),
  prompt_template TEXT NOT NULL,
  variables JSONB,  -- 变量定义
  use_count INT DEFAULT 0,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_merchant ( deleted_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 朋友圈文案 | 护肤品+轻松 | 朋友圈风格内容 | 待执行 |
| TC-002 | 小红书种草 | 美妆+种草 | 小红书风格 | 待执行 |
| TC-003 | 邮件营销 | B2B 邮件 | 正式专业 | 待执行 |
| TC-004 | 变量替换 | {{name}} | 正确替换 | 待执行 |
| TC-005 | 模板套用 | 套用模板 | 套用结果 | 待执行 |
| TC-006 | 批量生成 | 5 个产品 | 5 篇内容 | 待执行 |
| TC-007 | 风格迁移 | 同一产品多风格 | 风格差异明显 | 待执行 |
| TC-008 | 违规词过滤 | 含敏感词 | 拒绝/告警 | 待执行 |
| TC-009 | Prompt 注入 | 恶意提示 | 拒绝 | 待执行 |
| TC-010 | 频率限制 | 1 分钟 11 次 | 第 11 次拒绝 | 待执行 |
| TC-011 | LLM 超时 | 模拟超时 | 重试后降级 | 待执行 |
| TC-012 | 多模型路由 | 选用 deepseek | 调用 deepseek | 待执行 |
| TC-013 | 评分反馈 | 5 星 | 评分记录 | 待执行 |
| TC-014 | 收藏功能 | 收藏 | 收藏列表可见 | 待执行 |
| TC-015 | 历史查询 | 查询历史 | 正确返回 | 待执行 |
| TC-016 | 删除内容 | 软删除 | 列表不再显示 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| OpenAI Key | OPENAI_API_KEY | - | 加密存储 |
| 通义 Key | QWEN_API_KEY | - | |
| DeepSeek Key | DEEPSEEK_API_KEY | - | |
| 默认模型 | LLM_DEFAULT_MODEL | gpt-4 | |
| 单商户每分钟上限 | AI_RATE_LIMIT | 10 | |
| 最大输入 token | AI_MAX_INPUT_TOKENS | 4000 | |
| 合规词库 | COMPLIANCE_WORD_LIST | 内置 | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| LLM 调用失败 | > 5% | 钉钉 |
| 单商户调用异常 | 突增 10x | 自动暂停 |
| Token 用量超额 | 超过预算 80% | 邮件 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第九章功能清单(AI 内容生成)
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
