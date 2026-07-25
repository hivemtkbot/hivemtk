# 智能体 (aiAgent) — UI 交互清单

## 路由 /aiAgent/list  （59 个交互元素）

### button (54)
- [ ] Refresh
- [ ] New Agent
- [ ] Search
- [ ] Reset
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除
- [ ] 编辑
- [ ] 禁用
- [ ] 测试
- [ ] 绑定关系
- [ ] 删除

### select-ui (2)
- [ ] All Types
- [ ] All Statuses

### input (3)
- [ ] (无文本)
- [ ] (无文本)
- [ ] Search Name/Encoding

## 路由 /dialogueMemory/list  （8 个交互元素）

### button (6)
- [ ] Refresh
- [ ] Query
- [ ] 查看记忆
- [ ] 构建上下文
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

### input (2)
- [ ] Customer ID Search
- [ ] Page

## 路由 /intentRecognition/list  （24 个交互元素）

### switch (1)
- [ ] 启用

### button (11)
- [ ] Refresh
- [ ] Fill in the example
- [ ] 开始识别
- [ ] 批量识别(按行)
- [ ] 刷新
- [ ] 查询
- [ ] Go to previous page  `disabled`
- [ ] Go to next page
- [ ] Fill in the example
- [ ] 精细识别
- [ ] 查询

### input (7)
- [ ] 可选，用于客户画像归档
- [ ] (无文本)
- [ ] 可选，前序对话上下文，帮助提升识别准确率
- [ ] (无文本)
- [ ] 可选
- [ ] (无文本)
- [ ] 搜索意图/关键词

### select-ui (3)
- [ ] 可选，消息来源平台
- [ ] 意图类型筛选
- [ ] 大类筛选

### textarea (2)
- [ ] 输入客户的对话文本，如：这个多少钱？能便宜点吗？
- [ ] 输入客户消息，如：你们这个产品跟别家有什么区别？

## 路由 /objection/list  （10 个交互元素）

### button (6)
- [ ] Intelligent handling
- [ ] Refresh Categories
- [ ] 智能匹配
- [ ] 仅分类
- [ ] 清空
- [ ] 清空

### textarea (1)
- [ ] 输入客户的异议内容，例如：太贵了、再考虑一下、其他家更便宜...

### select-ui (1)
- [ ] 自动识别（可手动调整）

### input (1)
- [ ] (无文本)

### switch (1)
- [ ] (无文本)

## 路由 /persona/list  （2 个交互元素）

### button (1)
- [ ] Refresh

### input (1)
- [ ] Search Name/ID

## 路由 /sopAgent/list  （9 个交互元素）

### button (3)
- [ ] Refresh
- [ ] Query
- [ ] 创建 SOP

### input (2)
- [ ] Search SOP Name
- [ ] (无文本)

### select-ui (1)
- [ ] Status

### tab (3)
- [ ] SOP Management
- [ ] 执行监控
- [ ] 意图匹配测试

## 路由 /scriptTemplate/list  （4 个交互元素）

### button (1)
- [ ] New Calling Technique

### select-ui (1)
- [ ] My scene

### input (2)
- [ ] (无文本)
- [ ] Search Spells

## 路由 /llmRouting/list  （32 个交互元素）

### button (26)
- [ ] 新增模型
- [ ] 刷新
- [ ] 测试
- [ ] 启用
- [ ] 编辑
- [ ] 删除
- [ ] 测试
- [ ] 启用
- [ ] 编辑
- [ ] 删除
- [ ] 测试
- [ ] 禁用
- [ ] 编辑
- [ ] 删除
- [ ] 测试
- [ ] 启用
- [ ] 编辑
- [ ] 删除
- [ ] 测试
- [ ] 启用
- [ ] 编辑
- [ ] 删除
- [ ] 测试
- [ ] 启用
- [ ] 编辑
- [ ] 删除

### tab (6)
- [ ] 模型列表
- [ ] 场景路由配置
- [ ] Fallback 策略
- [ ] 成本统计
- [ ] 路由审计
- [ ] 分类统计与出域审计

## 路由 /confidence/panel  （9 个交互元素）

### input (2)
- [ ] Search session_id
- [ ] (无文本)

### button (3)
- [ ] Refresh
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

### select-ui (1)
- [ ] 20/page

### tab (3)
- [ ] Confidence signal
- [ ] 置信度校准
- [ ] 转人工阈值策略

## 路由 /humanize/panel  （8 个交互元素）

### button (3)
- [ ] Refresh
- [ ] Go to previous page  `disabled`
- [ ] Go to next page

### select-ui (1)
- [ ] 20/page

### input (1)
- [ ] (无文本)

### tab (3)
- [ ] <g id="Bold">Assessment Results</g>
- [ ] 销冠基线
- [ ] 低质样本

## 路由 /feedbackLoop/panel  （13 个交互元素）

### button (3)
- [ ] Refresh
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

### select-ui (3)
- [ ] 事件类型
- [ ] 信号 key
- [ ] 20/page

### input (3)
- [ ] (无文本)
- [ ] (无文本)
- [ ] (无文本)

### tab (4)
- [ ] Feedback Events
- [ ] 销冠对话
- [ ] Prompt 迭代
- [ ] Bandit A/B

## 路由 /asset-market  （16 个交互元素）

### select-ui (2)
- [ ] 全部
- [ ] 全部

### input (2)
- [ ] (无文本)
- [ ] (无文本)

### button (12)
- [ ] 查询
- [ ] 我的资产
- [ ] 免费试用
- [ ] 免费试用
- [ ] 免费试用
- [ ] 免费试用
- [ ] 免费试用
- [ ] 免费试用
- [ ] 免费试用
- [ ] 免费试用
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

## 路由 /asset-market/my-assets  （16 个交互元素）

### button (11)
- [ ] 去市场浏览
- [ ] 自建资产
- [ ] 同步日志
- [ ] 查询
- [ ] 同步
- [ ] 查看
- [ ] 上报使用
- [ ] 停用
- [ ] 删除
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

### select-ui (2)
- [ ] 全部
- [ ] 全部

### input (3)
- [ ] (无文本)
- [ ] (无文本)
- [ ] 搜索名称
