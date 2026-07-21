#!/bin/bash

# 获取token
TOKEN=$(curl -s -X POST http://localhost:8204/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "123456"}' | \
  grep -o '"token":"[^"]*' | cut -d'"' -f4)

echo "获取到的token: ${TOKEN:0:50}..."

# 获取当前无头模式状态
echo -e "\n📍 获取当前无头模式状态..."
curl -s -X GET http://localhost:8204/api/auto-reply/headless \
  -H "Authorization: Bearer $TOKEN" | jq .

# 设置无头模式为false（显示浏览器）
echo -e "\n📍 设置无头模式为false（显示浏览器）..."
curl -s -X POST http://localhost:8204/api/auto-reply/headless \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"headless": false}' | jq .

# 验证设置结果
echo -e "\n📍 验证设置结果..."
curl -s -X GET http://localhost:8204/api/auto-reply/headless \
  -H "Authorization: Bearer $TOKEN" | jq .

# 设置无头模式为true（后台运行）
echo -e "\n📍 设置无头模式为true（后台运行）..."
curl -s -X POST http://localhost:8204/api/auto-reply/headless \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"headless": true}' | jq .

echo -e "\n✅ 测试完成！"