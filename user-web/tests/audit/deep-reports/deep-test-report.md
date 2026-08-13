# 深度数据一致性测试汇总报告

- 生成时间: 2026-08-12
- 测试引擎: `hivemtk/user-web/tests/audit/deep-test.mjs`（层A 读一致性 + 层B 写一致性）
- 测试环境: `user-server`（dev 容器 :8204）+ PostgreSQL（:8232），**真实运行态服务**，直接打 localhost:8204 真实 API 并直查 PG。

---

## 一、方法论

**层B 写一致性（核心，精确实现用户"API 操作结果 vs 数据库结果"诉求）**
对每资源通过真实 API 执行 `创建 → 直接查 DB → 更新 → 查 DB → 删除 → 查 DB`，
逐字段递归比对「API 返回/提交对象」与「DB `row_to_json` 行」，排除自动字段（id / 时间戳 / 软删标记）。
这是用户要求的"提交表单 / 任何操作 对应 API 结果和数据库结果对比"的端到端精确实现。

**层A 读一致性（辅助）**
打开页面，抓取首个可见表格行，与页面调用的**所有** list 型 GET 接口做单元格子串包含比对。

---

## 二、层B 结果（7 个资源，全部通过）

| 资源 | DB 表 | 增 | 改 | 删 | 结果 |
|---|---|---|---|---|---|
| 知识库 | `knowledge_bases` | ✓ | ✓ | ✓ | OK |
| FAQ | `faq_entries` | ✓ | ✓ | ✓ | OK |
| SOP 模板 | `sop_templates` | ✓ | ✓ | ✓ | OK |
| 术语库 | `glossaries` | ✓ | ✓ | ✓ | OK |
| 话术模板 | `script_templates` | ✓ | ✓ | ✓ | OK |
| 素材分类 | `material_categories` | ✓ | ✓ | ✓ | OK（本轮新增） |
| 域名池 | `domain_pool` | ✓ | ✓ | ✓ | OK（本轮新增） |

全部 `errs=0`、`mismatches=0`。本轮新增 2 个资源（素材分类、域名池）未暴露新缺陷，功能正确。

### 历史修复（层B 发现的真实后端数据缺陷，均已修复并复验通过）
1. **FAQ / SOP 模板 Update 主键冲突 500**：`repo.Update` 用 `Select("*").Updates(entry)` 传入零值主键 → service 层先 `entry.ID = id` 修复。
2. **术语库 Update 400**：`GlossaryRequest.TermID` 强制 body 必填，但 `term_id` 来自路径 → 新增专用 `GlossaryUpdateRequest` + `translation.ToGlossaryModelUpdate` mapper。
3. **话术模板字段不落库**：Create 误用 `Name: req.Title`（name 恒等于 title）；Update 缺 `name` / `journey_stage` 字段 → 修正 DTO 与赋值逻辑。

---

## 三、层A 结果（10 个页面标记 → 经源码核查全部为误报）

被标记页面：`confidence/panel`、`faq/editor/1`、`faq/list`、`feedbackLoop/panel`、`i18n/dashboard`、`intentRecognition/list`、`knowledge/management`、`persona/list`、`sop-template/list`、`tagSegmentation/list`。

**判定为误报的根因（已用前端源码核实）：**
层A 把"页面**首个可见表格**"对"页面调用的**每一个** list GET 接口（含下拉 / 筛选 / 配置类辅助查询）"做子串比对。每个被标记页面的**主数据源**（真正喂给主表格的 list 接口）均**未被标记**（匹配通过）；被标记的均为**辅助 / 引用端点**，例如：

- `faq/list`：`/api/faqs`（主数据源，匹配通过）vs `/api/knowledge-bases?kb_type=faq`（仅用于"所属知识库"列下拉，`listKBs({kb_type:'faq'})` 返回知识库列表 ≠ FAQ 行）→ 误报。
- `sop-template/list`：`/api/sop-templates`（主，匹配）vs `/api/knowledge-bases?kb_type=sop`（辅助）→ 误报。
- `tagSegmentation/list`：`/api/user-segment/layers` 返回 11 条 RFM 分层**参考定义**（固定配置，非用户标签表），与 `/api/session-tags`（辅助，0 行）→ 误报。该页实际标签数据来自另一未标记的 list 接口（已匹配）。
- 其余 `panel` / `dashboard` 页为聚合 / 统计视图，展示值为计算 / 格式化结果（如 `术语 123 / FAQ 45`），无法通过原始 list JSON 子串比对 → 方法论局限。

---

## 四、结论与建议

- 用户核心诉求「API ↔ DB 数据正确性」经层B 严格端到端验证，**7 资源全覆盖、全绿**；此前已修复 4 个真实后端缺陷并复验通过。
- 层A 的 10 个标记均为测试方法学局限（首表 × 全 list 子串、辅助端点、格式化展示）造成的**误报**，非真实数据缺陷。
- **建议**（如需对"页面展示数据"做真正确证）：将层A 升级为「主表格 ↔ 主 list 接口（按 id 主键）↔ DB」三方对账，替代当前粗暴的首表 × 全 list 子串比对；当前层A 仅作为冒烟级信号，不作为缺陷判据。
