# R41 闭环报告（2026-08-29 · 第三轮循环：多角色全端模拟测试）

> 采纳标准（用户钦定）：**站在用户角度，功能是否需要，是否有必要 — 其他不考虑**

## 六步执行摘要

### Step1 全量复扫
- 170 前端路由 × 77 核心页面三维检测（HTTP状态/console错误/页面渲染）：**66/77 PASS**，发现 4 类真实缺陷

### Step2 调研
- 本轮以模拟测试发现为导向（后端代码+日志+Playwright 三源交叉定位根因），无需外部调研

### Step3+4 论证（用户视角裁决）
| 发现 | 根因 | 裁决 |
|------|------|------|
| 8+ 页面 502 错误流（message/latest） | platformData 先写 502 落盘，R40 的 SUCCESS 补写无效 → 双响应体拼接 | **修**：轮询端点改用不写响应内核 |
| customerEvent 页 404 + N+1 | 页面全客户逐个拉取；脏 conv-id（含 `%2F`）触发 Gin 解码 404（R32 遗留） | **修**：新增全局分页端点，根除两病 |
| douyinCard 裂图 | demo 数据占位外链 img.example.com 不可达（BACKLOG 已记） | **修**：5 平台卡片 @error 兜底占位 |
| 坐席工作台 WS 拒连 | 默认 Origin 白名单(3000/8080)与项目实际端口(8212)不匹配 | **修**：默认白名单补齐本地端口 |

### Step5 开发（4 项修复）
1. **platform.go 重构**：platformData 拆为 `platformDataRaw`（不写响应内核，返回 error）+ 错误写回层；GetLatestMessage 改用 Raw 静默降级（不触碰其余 30+ 平台代理端点语义）
2. **customer-events 全局流**：repository.ListGlobal + controller.ListGlobal + GET /api/customer-events/list 五层新端点；前端 loadEvents 从 N+1（全客户×50条并发）改为单次分页请求
3. **卡片图片兜底**：douyin/kuaishou/xiaohongshu/xianyu/tiktok 五平台 List.vue el-image 加 @error 内置 SVG 占位
4. **WS Origin 白名单**：DefaultAllowedWSOrigins 补 5173/8212/8204（含 127.0.0.1 形式），本地开发坐席 WS 恢复可用

### Step6 验证
- 复测 11 个问题页面：**11/11 PASS**（douyinCard 仅剩 @error 兜底后的占位渲染、ws 零报错）
- WS 连接专项：customerSession 页 WebSocket 零错误 ✅
- message/latest 直接 curl：200 `{"code":"SUCCESS"}`（原 502+拼接体）✅
- global-events 端点：total=2 rows=2 ✅
- 回归：vitest 174/174、vite build ✓、config/websocket 单测绿、仓库 build+vet 绿

## 已提交
commit 见 git log（fix(r41)）→ Gitee + GitHub 双远端

## 遗留
- img.example.com 脏 demo 数据仍存 DB（前端已优雅兜底；数据清理属一次性运维动作）
- WS origin 白名单默认值放宽到本地回环端口——生产部署仍应通过 ALLOWED_WS_ORIGINS 显式收紧（注释已标注）
