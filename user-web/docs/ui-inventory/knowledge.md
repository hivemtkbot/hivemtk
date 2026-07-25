# 知识中心 (knowledge) — UI 交互清单

## 路由 /knowledge/management  （47 个交互元素）

### select-ui (4)
- [ ] Select Products
- [ ] Status oEmbed
- [ ] Source Type
- [ ] 20/page

### input (6)
- [ ] (无文本)
- [ ] (无文本)
- [ ] (无文本)
- [ ] Search Title
- [ ] (无文本)
- [ ] Page

### button (29)
- [ ] Search
- [ ] Import Files
- [ ] Refresh
- [ ] 详情
- [ ] 重建索引
- [ ] 删除
- [ ] 详情
- [ ] 重建索引
- [ ] 删除
- [ ] 详情
- [ ] 重建索引
- [ ] 删除
- [ ] 详情
- [ ] 重建索引
- [ ] 删除
- [ ] 详情
- [ ] 重建索引
- [ ] 删除
- [ ] 详情
- [ ] 重建索引
- [ ] 删除
- [ ] 详情
- [ ] 重建索引
- [ ] 删除
- [ ] 详情
- [ ] 重建索引
- [ ] 删除
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

### link (8)
- [ ] HiveMTK 项目概述与定位
- [ ] HiveMTK 开源信息与协议
- [ ] HiveMTK 部署指南
- [ ] HiveMTK 运维手册
- [ ] HiveMTK 架构说明
- [ ] HiveMTK 功能模块
- [ ] HiveMTK 资产市场与 AI 智能体
- [ ] HiveMTK 常见问题 FAQ

## 路由 /knowledge/tokens  （10 个交互元素）

### select-ui (2)
- [ ] 按产品筛选
- [ ] 选择产品

### input (4)
- [ ] (无文本)
- [ ] 如:CRM 推送
- [ ] (无文本)
- [ ] 不填则永不过期

### button (2)
- [ ] 刷新
- [ ] 创建

### checkbox (2)
- [ ] 只读
- [ ] 可写

## 路由 /knowledge/playground  （9 个交互元素）

### select-ui (2)
- [ ] 选择产品
- [ ] 选择或输入标签

### input (3)
- [ ] (无文本)
- [ ] 可选,如:售后/产品
- [ ] (无文本)

### textarea (1)
- [ ] 输入用户问题,例如:如何申请退款?

### button (1)
- [ ] 开始检索  `disabled`

### checkbox (2)
- [ ] 启用三级缓存
- [ ] 启用重排序

## 路由 /knowledge/chunks  （8 个交互元素）

### select-ui (2)
- [ ] 0
- [ ] 20/page

### input (3)
- [ ] (无文本)
- [ ] (无文本)
- [ ] Page

### button (3)
- [ ] Refresh Segments  `disabled`
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

## 路由 /knowledge/external  （14 个交互元素）

### radio (4)
- [ ] custom
- [ ] feishu
- [ ] notion
- [ ] dingtalk

### select-ui (2)
- [ ] 选择产品
- [ ] 按产品筛选

### input (3)
- [ ] (无文本)
- [ ] 请先到「API Token 管理」创建
- [ ] (无文本)

### textarea (1)
- [ ] JSON 数组,例如: [{"title": "FAQ1", "content": "...", "category": "售后", "tags": ["退款"

### checkbox (1)
- [ ] 同步返回(否则异步)

### button (3)
- [ ] 载入模板
- [ ] 提交导入  `disabled`
- [ ] 刷新

## 路由 /knowledge/openapi  （4 个交互元素）

### select-ui (1)
- [ ] 选择产品(可选)

### input (1)
- [ ] (无文本)

### button (2)
- [ ] 刷新
- [ ] 新建数据源

## 路由 /knowledge/statistics  （9 个交互元素）

### select-ui (2)
- [ ] Select a product (optional)
- [ ] 近 30 天

### input (2)
- [ ] (无文本)
- [ ] (无文本)

### button (1)
- [ ] Refresh

### radio (4)
- [ ] trend
- [ ] source
- [ ] category
- [ ] top

## 路由 /knowledge/feedbacks  （12 个交互元素）

### select-ui (3)
- [ ] 选择产品
- [ ] 评价
- [ ] 20/page

### input (5)
- [ ] (无文本)
- [ ] (无文本)
- [ ] 搜索查询文本
- [ ] (无文本)
- [ ] Page

### button (4)
- [ ] 搜索
- [ ] 刷新
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

## 路由 /knowledge/batch-import  （10 个交互元素）

### select-ui (1)
- [ ] 选择产品

### input (1)
- [ ] (无文本)

### radio (3)
- [ ] auto
- [ ] csv
- [ ] json

### upload (1)
- [ ] 将文件拖到此处,或点击上传

### button (2)
- [ ] 预览解析结果  `disabled`
- [ ] 确认导入 (0)  `disabled`

### tab (2)
- [ ] 文件上传
- [ ] JSON 粘贴

## 路由 /templateMarket/list  （2 个交互元素）

### button (2)
- [ ] Return to previous
- [ ] Back to Home

## 路由 /system/rag-overview  （0 个交互元素）

## 路由 /system/rag-product-config  （11 个交互元素）

### button (6)
- [ ] Link New Product
- [ ] 编辑
- [ ] 停用
- [ ] 删除
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

### select-ui (1)
- [ ] 20/page

### input (2)
- [ ] (无文本)
- [ ] Page

### tab (2)
- [ ] Rag Product Management
- [ ] Account Configuration

## 路由 /system/rag-product  （9 个交互元素）

### button (6)
- [ ] Link New Product
- [ ] 编辑
- [ ] 停用
- [ ] 删除
- [ ] Go to previous page  `disabled`
- [ ] Go to next page  `disabled`

### select-ui (1)
- [ ] 20/page

### input (2)
- [ ] (无文本)
- [ ] Page

## 路由 /system/rag-account  （13 个交互元素）

### select-ui (2)
- [ ] 选择平台
- [ ] 选择RAG产品

### input (4)
- [ ] (无文本)
- [ ] 输入平台账号ID
- [ ] (无文本)
- [ ] 1000

### other (2)
- [ ] decrease number
- [ ] increase number

### switch (2)
- [ ] 自动回复
- [ ] RAG智能体  `disabled`

### button (3)
- [ ] 添加规则
- [ ] 保存配置
- [ ] 重置

## 路由 /aiContent/list  （2 个交互元素）

### button (2)
- [ ] Return to previous
- [ ] Back to Home
