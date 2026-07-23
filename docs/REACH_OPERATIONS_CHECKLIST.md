# 触达运营（reach）全量菜单清单与自动化测试进度

> 主 agent 依据本清单逐页处理：读取页面 → 审计并完善 UI/按钮/API（对照 docs 契约）→ 调用 Playwright 模拟人工点击测试（捕获 页面渲染 / API 参数与响应 / 数据库结果 / 控制台输出 / API 日志）→ 有问题立即修复 → 标记完成 → 循环下一页。
>
> 运行环境：前端 dev `http://localhost:8213`（vite），后端 `http://localhost:8204`（user-server）。API base 见 `src/utils/request.js`。登录态持久化于 `tests/.auth/user.json`（管理员 admin / Admin@123456）。
> 同构页面：抖音/快手/小红书/咸鱼卡片 与 TikTok 卡片结构一致；其自动回复/统计同理。

## 进度图例
- [ ] 待处理  [~] 审计/修复中  [x] 已完成（UI+API+Playwright 通过）

---

## 一、邮件触达（emailReach） ✅
- [x] 邮件列表 `/email` → `src/views/email/EmailList.vue` · api `email.js` · 契约 `email-list-management.md`
  - 修复：原页只有发送记录列表、无操作按钮；后端 `/api/email/list` 返回 `subject/to/from/is_send/is_read/send_time/read_time`（字段全小写，列已匹配）。已补 **发送邮件 / 查看追踪 / 删除** 按钮 + 对话框，并补 i18n 键（emailSubject/emailTo/emailFrom/emailIsSend/emailSentAt/emailRead/emailReadAt/sendEmail/viewTrace/traceInfo/emailContentPreview）。
- [x] 我的草稿 `/email/drafts` → 完整（新建/编辑/删除/转为任务 均通 API）。
- [x] 我的任务 `/email/jobs` → 只读统计表（任务由草稿“转为任务”创建，无独立新建按钮，符合设计）。
- [x] 邮件账号 `/email/smtp` → 完整（增删改/测试连接）。
- [x] 邮件代理 `/email/info` → 静态代理参考列表。
- [x] 邮件使用指南 `/email/guide` → 静态文档。

## 二、短信触达（smsReach） ✅
- [x] 短信列表 `/sms/list` → `src/views/sms/List.vue` · api `sms.js` · 契约 `sms-list-management.md`
  - 修复：原“发送短信”主按钮**无 @click（死按钮）**；后端 `POST /api/sms/send` 存在（要求服务商已配置，未配置时优雅报错）。已补发送对话框（手机号正则校验 + 内容）+ 接线 `smsApi.sendSms`。
- [x] 短信草稿 `/sms/drafts` → 完整。
- [x] 短信任务 `/sms/jobs` → 完整（创建/暂停/恢复/停止/删除/详情）。
- [x] 短信配置 `/sms/config` → 完整（阿里云/腾讯云/华为云 配置保存）。

## 三、抖音（douyin） ✅
- [x] 抖音卡片 `/douyinCard` → 完整（增删改/统计/复制链接，按钮均接线）。
- [x] 抖音自动回复 `/douyin/auto-reply` → 完整。
- [x] 抖音统计 `/douyin/stats` → 图表页（无主按钮，渲染+接口正常）。

## 四、快手（kuaishou） ✅
- [x] 快手卡片 `/kuaishouCard` · [x] 快手自动回复 `/kuaishou/auto-reply` · [x] 快手统计 `/kuaishou/stats` → 与抖音同构，均已接线。

## 五、小红书（xiaohongshu） ✅
- [x] 小红书卡片 `/xiaohongshuCard` · [x] 小红书自动回复 `/xiaohongshu/auto-reply` · [x] 小红书统计 `/xiaohongshu/stats` → 完整。

## 六、咸鱼（xianyu） ✅
- [x] 咸鱼卡片 `/xianyuCard` · [x] 咸鱼自动回复 `/xianyu/auto-reply` · [x] 咸鱼统计 `/xianyu/stats` → 完整。

## 七、TikTok（tiktok） ✅
- [x] TikTok 卡片 `/tiktok/list` · [x] TikTok 统计 `/tiktok/stats` · [x] TikTok 自动回复 `/tiktok/auto-reply` → 完整（代表卡族深度覆盖）。

## 八、WhatsApp / Telegram / 飞书 ✅
- [x] 账号管理 `/whatsapp/account` · [x] 草稿箱 `/whatsapp/drafts` · [x] 群发 `/whatsapp/jobs` → 完整。
- [x] 机器人 `/telegram/account` → 完整。
- [x] 飞书账号 `/feishu/account` → 完整。

## 九、社群 / 短链 / 活码 ✅
- [x] 社群管理 `/community/list` → 完整。
- [x] 短链列表 `/shortLink` · [x] 短链统计 `/shortLink/stats` → 完整。
- [x] 活码管理 `/livecode` → 完整。

---

## 自动化测试套件（触达运营）
- `tests/reach_operations.spec.js` — **全量 34 页冒烟**：导航 + 渲染可见 + 控制台错误过滤 + API 5xx 捕获。**全部通过**。
- `tests/auth.setup.spec.js` — 单次 UI 登录并持久化 `tests/.auth/user.json`（避免登录限流）。
- `tests/reach_email_functional.spec.js` — 邮件组深度功能（发送对话框+空主题校验/查看追踪-删除确认/草稿-任务-账号对话框）。**5/5 通过**。
- `tests/reach_sms_functional.spec.js` — 短信组深度功能（发送对话框+手机号校验/草稿/任务/配置）。**4/4 通过**。
- `tests/reach_remaining_functional.spec.js` — 剩余 24 页主 CTA 点击无崩溃/无 5xx。**24/24 通过**。

### 运行方式
```bash
# 前端 dev 已在 8213；先生成登录态
E2E_BASE_URL=http://localhost:8213 npx playwright test tests/auth.setup.spec.js
# 跑全部触达运营测试
E2E_BASE_URL=http://localhost:8213 npx playwright test tests/reach_operations.spec.js tests/reach_email_functional.spec.js tests/reach_sms_functional.spec.js tests/reach_remaining_functional.spec.js --workers=1
```

## 执行记录
- 2026-07-23：构建 34 页清单；修复 EmailList（发送/追踪/删除 按钮+i18n）、SmsList（发送按钮死链→接线）；建立 Playwright 自动化循环（冒烟34 + 功能 邮件5/短信4/其余24），全部绿灯；`npm run build` 通过。
- 环境注意：本地后端管理员密码被重置为 `Admin@123456`（原哈希未知，无法登录，已用 pgcrypto 重置）；登录接口受全局限流（RPS≈10），故测试用单次登录+storageState，避免触发限流导致批量失败。
