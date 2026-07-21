# 营销系统API测试套件

## 🎯 概述

这是一个完整的营销系统API测试套件，专门用于验证所有API功能，特别是修复后的ID验证、参数处理和必填字段验证。

## 📁 测试脚本文件

### 1. `test-api-complete.sh` (推荐)
- **功能**: 完整的API测试套件，包含用户注册、登录认证、全量API测试
- **特点**: 彩色输出、详细日志、错误统计、修复验证
- **使用**: `bash test-api-complete.sh`

### 2. `complete-api-test-suite.sh`
- **功能**: 重点验证修复的API功能
- **特点**: 专门的ID验证修复测试、参数处理验证
- **使用**: `bash complete-api-test-suite.sh`

### 3. `test-comprehensive-api.js`
- **功能**: Node.js实现的API测试
- **特点**: 异步处理、Promise链、详细错误捕获
- **使用**: `node test-comprehensive-api.js`

### 4. `test-fixed-apis-final.js`
- **功能**: 专门验证修复的API功能
- **特点**: 针对性强、修复验证完整
- **使用**: `node test-fixed-apis-final.js`

### 5. `test-guide.sh`
- **功能**: 测试套件使用指南
- **特点**: 彩色显示、使用说明、快速参考
- **使用**: `bash test-guide.sh`

## 🔧 测试前准备

### 1. 启动API服务
```bash
cd /Users/xiaofang/Documents/www/php/tggate/marketing/cmd/api
go run main.go
```

### 2. 确保服务运行
- 服务地址: `http://localhost:8204`
- 默认管理员账户: `admin/123456`

### 3. 创建测试结果目录
```bash
mkdir -p /tmp/api-test-results
```

## 🧪 测试覆盖范围

### ✅ 用户认证系统
- 管理员登录
- 用户注册
- JWT令牌管理
- 权限验证

### ✅ 账户管理API (ID验证修复重点)
- 账户创建
- 账户更新 (ID一致性验证)
- 账户列表查询
- UUID格式验证
- URI参数与JSON请求体ID匹配验证

### ✅ 多平台卡片管理
- 抖音卡片API
- 快手卡片API
- 小红书卡片API
- 闲鱼卡片API
- 卡片ID处理优化验证

### ✅ 短链接管理API
- 短链接创建
- 短链接统计 (参数处理修复)
- 短链接更新
- 路由参数优化

### ✅ 活码管理API
- 活码创建
- 必填字段验证
- 字段缺失错误处理

### ✅ 邮件系统API
- 邮件列表管理
- SMTP配置
- 邮件任务
- 邮件草稿

### ✅ OBS云存储配置
- OBS配置列表
- OBS配置创建
- 云存储参数验证

### ✅ 系统管理与监控
- 系统日志
- 系统统计
- 系统配置
- 管理员配置

### ✅ 其他核心功能
- 线索管理
- 订单管理
- 素材管理
- 平台统计
- 消息管理

## 🎯 重点修复验证

### 1. 账户更新API ID验证修复
```go
// 从URI参数获取ID并验证与JSON中的ID一致
idStr := ctx.Param("id")
if idStr == "" {
    ctx.JSON(http.StatusBadRequest, gin.H{"error": "账户ID不能为空"})
    return
}

accountID, err := uuid.Parse(idStr)
if err != nil {
    ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的账户ID格式"})
    return
}
```

### 2. 卡片管理ID处理优化
- 统一各平台卡片的ID验证逻辑
- 确保URI参数与请求体ID一致性
- 完善的错误提示信息

### 3. 短链接统计API参数处理
- 路由参数从查询参数改为URI参数
- 修复统计API的参数传递问题
- 路径: `/short-link/:id/stats`

### 4. 活码管理必填字段验证
- 完善必填字段验证逻辑
- 字段缺失时的错误提示
- 参数完整性检查

### 5. 新增系统管理路由
- 邮件账户管理: `/email/accounts`
- 邮件模板管理: `/email/templates`
- 系统日志: `/system/logs`
- 系统统计: `/system/stats`
- 系统配置: `/system/config`

## 📊 测试结果查看

### 实时查看测试进度
```bash
tail -f /tmp/api-test-results/complete_marketing_api_test_*.log
```

### 查看最新测试报告
```bash
ls -la /tmp/api-test-results/ | head -10
```

### 查看具体报告内容
```bash
cat /tmp/api-test-results/complete_marketing_api_test_*.log
```

## 🚀 快速开始

### 运行完整测试套件
```bash
bash test-api-complete.sh
```

### 运行修复验证测试
```bash
bash complete-api-test-suite.sh
```

### 查看使用指南
```bash
bash test-guide.sh
```

## 📈 测试统计

测试脚本会自动统计：
- ✅ 成功测试数量
- ❌ 失败测试数量
- 📊 测试成功率
- 🕐 测试执行时间
- 📋 详细的错误信息

## 🔍 问题排查

### 常见错误
1. **服务未启动**: 确保API服务正在运行
2. **端口占用**: 检查8086端口是否被占用
3. **认证失败**: 验证管理员账户密码
4. **网络错误**: 检查网络连接和服务地址

### 调试建议
1. 查看详细日志文件
2. 检查API服务日志
3. 验证数据库连接
4. 确认路由配置正确

## 💡 使用建议

1. **日常测试**: 使用 `test-api-complete.sh` 进行全面测试
2. **修复验证**: 使用 `complete-api-test-suite.sh` 专门验证修复
3. **快速检查**: 使用 `test-comprehensive-api.js` 进行快速测试
4. **问题排查**: 查看详细日志文件分析问题

## 🎨 输出示例

```
╔══════════════════════════════════════════════════════════════╗
║                营销系统API完整测试套件                      ║
║           Marketing System API Test Suite                  ║
╚══════════════════════════════════════════════════════════════╝

测试时间: 2025-12-24 11:30:00
测试目标: http://localhost:8204
结果文件: /tmp/api-test-results/complete_marketing_api_test_20251224_113000.log

🧪 系统健康检查
=========================================
✅ [SUCCESS] API服务运行正常

🧪 管理员认证登录  
=========================================
✅ [SUCCESS] 管理员登录 - HTTP 200
✅ [SUCCESS] 管理员令牌获取成功

🧪 账户管理API测试 - ID验证修复验证
=========================================
✅ [SUCCESS] 获取账户列表 - HTTP 200
✅ [SUCCESS] 创建测试账户 - HTTP 200
✅ [SUCCESS] 账户更新(ID一致) - HTTP 200
✅ [SUCCESS] 账户更新(无效ID格式) - HTTP 400
✅ [SUCCESS] 账户更新(ID不一致) - HTTP 400

📊 测试结果汇总
=========================================
总测试数: 45
✅ 成功: 42
❌ 失败: 3
📈 成功率: 93%

🎉 恭喜！大部分API测试通过，系统运行正常！
```

## 📞 支持

如有问题，请查看：
1. 详细测试日志文件
2. API服务运行日志
3. 系统配置文件
4. 数据库连接状态

---

**测试套件版本**: 1.0.0  
**最后更新**: 2025-12-24  
**适用系统**: 营销系统API v1.0+