#!/bin/bash

# 测试商户账户无头模式设置
# 该脚本测试通过商户账户设置各平台的自动回复无头模式

API_BASE="http://localhost:8204/api"
AUTH_HEADER="Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6ImFkbWluIiwiZXhwIjoxNzM1MTg5MTQ5fQ.8VRU5q2a2Y5kF8U8E7z1t3pL6mN9qR2sT5uW8xY1bC4"

echo "=== 测试商户账户无头模式设置 ==="
echo

# 1. 创建商户账户（如果不存在）
echo "1. 创建测试商户账户..."
curl -s -X POST "$API_BASE/account" \
  -H "$AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d '{
    "tg_bot_token": "test_bot_token",
    "price": "100",
    "group_id": 123456,
    "epay_pid": "test_pid",
    "epay_key": "test_key",
    "epay_pay_type": "alipay",
    "epay_query_url": "https://api.example.com/query",
    "epay_url": "https://pay.example.com",
    "proxy_enable_proxy": false,
    "douyin_headless": false,
    "kuaishou_headless": true,
    "xiaohongshu_headless": false,
    "xianyu_headless": true
  }' | jq '.'

echo
echo "2. 获取商户账户列表..."
curl -s -X GET "$API_BASE/account/list" \
  -H "$AUTH_HEADER" | jq '.'

echo
echo "3. 获取当前无头模式设置..."
curl -s -X GET "$API_BASE/auto-reply/headless" \
  -H "$AUTH_HEADER" | jq '.'

echo
echo "4. 修改抖音平台无头模式为true（后台运行）..."
curl -s -X POST "$API_BASE/auto-reply/headless" \
  -H "$AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d '{
    "platform": "douyin",
    "headless": true
  }' | jq '.'

echo
echo "5. 修改小红书平台无头模式为true（后台运行）..."
curl -s -X POST "$API_BASE/auto-reply/headless" \
  -H "$AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d '{
    "platform": "xiaohongshu",
    "headless": true
  }' | jq '.'

echo
echo "6. 再次获取无头模式设置确认修改..."
curl -s -X GET "$API_BASE/auto-reply/headless" \
  -H "$AUTH_HEADER" | jq '.'

echo
echo "=== 测试完成 ==="
echo "注意：商户账户的无头模式设置将在下次启动自动回复机器人时生效"