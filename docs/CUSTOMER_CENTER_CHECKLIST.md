# 客户中心（Customer Center）页面清单

> 来源：`hivemtk/user-web/src/layout/Layout.vue` 中 `key: 'customer'` 顶级菜单（第 193-217 行）。
> 范围：仅「客户中心」顶级菜单下的叶子页面。注：`客服会话 /customerSession/list` 属于「智能体 → 被动应答」，不在本清单内；`/oneid/conflicts`（身份冲突解决）后端未接通已从菜单隐藏。

| # | 菜单 key | 名称 | 路由 | 视图文件 | 状态 |
|---|----------|------|------|----------|------|
| 1 | clueList | 线索列表 | /clue/list | views/clue/List.vue | 待完善/测试 |
| 2 | clueStatistics | 线索统计 | /clue/statistics | views/clue/Statistics.vue | 待完善/测试 |
| 3 | customer360 | 客户 360 | /customer360/list | views/customer360/List.vue | 待完善/测试 |
| 4 | customerEvent | 客户事件 | /customerEvent/list | views/customerEvent/List.vue | 待完善/测试 |
| 5 | tagSegmentation | 标签分层 | /tagSegmentation/list | views/tagSegmentation/List.vue | 待完善/测试 |
| 6 | userSegment | 用户分层 RFM | /userSegment/list | views/userSegment/List.vue | 待完善/测试 |
| 7 | unifiedMessage | 统一消息 | /unifiedMessage/list | views/unifiedMessage/List.vue | 待完善/测试 |
| 8 | oneidList | OneID 列表 | /oneid/list | views/oneid/List.vue | 待完善/测试 |

## 执行计划

- 第一步：本清单（已完成）
- 第二步：逐页审查 `views/<page>` + `src/api/<page>.js` + `user-server` 后端 controller/service/route，补全缺失的 UI 元素、按钮、API 调用
- 第三~五步：主 Agent 读取清单逐个页面，用 Playwright 子流程模拟人工点击，结合 API 参数 / 页面渲染 / 数据库结果 / 控制台输出 / API 日志，100% 覆盖测试，发现问题即时修复，循环至全部完成

## 每页测试覆盖矩阵（模板）

每个页面需覆盖：
- [ ] 页面加载渲染（无白屏、无控制台错误）
- [ ] 列表/表格数据加载（分页、筛选、排序）
- [ ] 新建/编辑/删除 等写操作按钮（若有）
- [ ] 搜索/筛选/导出 等查询按钮
- [ ] 详情抽屉/弹窗 打开与关闭
- [ ] API 请求参数正确、响应正确解析、数据库落库正确
- [ ] 边界与异常（空数据、权限、网络错误）
