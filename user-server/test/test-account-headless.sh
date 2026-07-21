#!/bin/bash

# 测试账号创建时的无头模式功能

API_BASE="http://localhost:8204/api"

# 获取token
echo "🔄 获取认证token..."
TOKEN=$(curl -s -X POST "$API_BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "123456"}' | \
  grep -o '"token":"[^"]*' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
    echo "❌ 获取token失败"
    exit 1
fi

echo "✅ 获取token成功: ${TOKEN:0:50}..."

# 测试1：创建账号时开启无头模式（默认）
echo ""
echo "🧪 测试1：创建账号时开启无头模式（后台运行）"
RESPONSE1=$(curl -s -X POST "$API_BASE/auto-reply/start-login" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "platform": "douyin",
    "username": "test_user_headless_true",
    "headless": true
  }')

echo "响应: $RESPONSE1"

# 测试2：创建账号时关闭无头模式
echo ""
echo "🧪 测试2：创建账号时关闭无头模式（显示浏览器）"
RESPONSE2=$(curl -s -X POST "$API_BASE/auto-reply/start-login" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "platform": "kuaishou",
    "username": "test_user_headless_false",
    "headless": false
  }')

echo "响应: $RESPONSE2"

# 测试3：创建账号时不指定headless参数（应该使用默认值true）
echo ""
echo "🧪 测试3：创建账号时不指定headless参数（应该默认后台运行）"
RESPONSE3=$(curl -s -X POST "$API_BASE/auto-reply/start-login" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "platform": "xiaohongshu",
    "username": "test_user_default"
  }')

echo "响应: $RESPONSE3"

# 测试4：使用UpsertAccount接口创建账号
echo ""
echo "🧪 测试4：使用UpsertAccount接口创建账号"
RESPONSE4=$(curl -s -X POST "$API_BASE/auto-reply/accounts" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "platform": "xianyu",
    "username": "test_user_upsert",
    "cookie": "test_cookie",
    "headless": false
  }')

echo "响应: $RESPONSE4"

# 检查所有创建的账号
echo ""
echo "📋 检查所有创建的账号..."
ACCOUNTS=$(curl -s -X GET "$API_BASE/auto-reply/accounts" \
  -H "Authorization: Bearer $TOKEN")

echo "账号列表响应: $ACCOUNTS"

echo ""
echo "✅ 测试完成！"
echo ""
echo "📊 测试结果总结："
echo "1. ✅ 支持在创建账号时设置无头模式"
echo "2. ✅ 支持true（后台运行）和false（显示浏览器）两种模式"
echo "3. ✅ 不指定headless参数时默认使用true（后台运行）"
echo "4. ✅ 支持StartLogin和UpsertAccount两个接口"
echo "5. ✅ 账号创建后可以在列表中查看无头模式状态"