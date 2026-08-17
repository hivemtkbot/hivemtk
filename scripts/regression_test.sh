#!/bin/bash
set -e

BASE="http://localhost:8204/api"
AUTH=(-H "Authorization: Bearer dummy")

echo "======================================"
echo " 聚焦修复代码的回归测试"
echo "======================================"

# 1. Login
echo -e "\n[Step 1] 登录获取 Token"
TOKEN=$(curl -s -X POST -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"TestPwd_2026!"}' \
    "$BASE/auth/login" | python3 -c "import json,sys; print(json.load(sys.stdin).get('data',{}).get('token',''))")
if [ -z "$TOKEN" ]; then
    echo "  ❌ 登录失败"
    exit 1
fi
AUTH=(-H "Authorization: Bearer $TOKEN")
echo "  ✅ 登录成功"

# --------------------------------------------------
# Test 2: BindChannel GORM 修复
# --------------------------------------------------
echo -e "\n[Step 2] 回归测试: BindChannel GORM 修复"
echo "  验证: 使用 model.CustomerChannel 结构体代替 map[string]any"
echo "        解决 'upsert failed: model value required' 错误"

# 2a. 第一次绑定 (CREATE) - 使用已存在的客户
CUSTOMER_ID="acf15f7a-9b41-4de8-bc86-ad7827693162"
echo "  -> 执行第一次绑定 (CREATE)"
RESP1=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d "{\"customer_id\":\"$CUSTOMER_ID\",\"channel\":\"telegram\",\"channel_user_id\":\"123456\",\"channel_name\":\"Regression Test TG\"}" \
    "$BASE/channels/bind")
CODE1=$(echo "$RESP1" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)

if [ "$CODE1" = "SUCCESS" ] || [ "$CODE1" = "0" ]; then
    echo "  ✅ 2a. 首次绑定成功 (code=$CODE1)"
else
    echo "  ❌ 2a. 首次绑定失败 (code=$CODE1, resp=$RESP1)"
    echo "      这表明 GORM FirstOrCreate + Assign 修复未生效"
    exit 1
fi

# 2b. 第二次绑定 (UPDATE) - 验证幂等性
echo "  -> 执行第二次绑定 (UPDATE - 幂等性)"
RESP2=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d "{\"customer_id\":\"$CUSTOMER_ID\",\"channel\":\"telegram\",\"channel_user_id\":\"789012\",\"channel_name\":\"Updated TG Name\"}" \
    "$BASE/channels/bind")
CODE2=$(echo "$RESP2" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)

if [ "$CODE2" = "SUCCESS" ] || [ "$CODE2" = "0" ]; then
    echo "  ✅ 2b. 幂等更新成功 (code=$CODE2)"
else
    echo "  ❌ 2b. 幂等更新失败 (code=$CODE2, resp=$RESP2)"
fi

# 2c. 验证数据库中的数据确实被更新了
echo "  -> 验证数据库数据一致性"
sleep 0.5
DB_RESULT=$(docker exec mtk-postgres psql -U admin -d user_db -p 8202 -t -A \
    -c "SELECT channel_user_id FROM customer_channels WHERE one_id = (SELECT unified_id FROM customers WHERE id = '$CUSTOMER_ID') AND channel = 'telegram';" 2>/dev/null)
if echo "$DB_RESULT" | grep -q "789012"; then
    echo "  ✅ 2c. 数据库数据已更新 ($DB_RESULT)"
else
    echo "  ❌ 2c. 数据库数据未更新 (got: $DB_RESULT)"
fi

# --------------------------------------------------
# Test 3: pickChannel 逻辑修复 + dry_run 修复
# --------------------------------------------------
echo -e "\n[Step 3] 回归测试: pickChannel 逻辑 + dry_run 修复"
echo "  验证: dry_run=true 时跳过账号检查，正确选中客户有身份的渠道"

# 使用已存在的有 email 的客户
echo "  -> 测试 dry_run 选渠道 (客户有 email: full_e2e@test.com)"
REACH_RESP=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"content":"回归测试消息","dry_run":true}' \
    "$BASE/reach/proactive/customer/$CUSTOMER_ID")
REACH_CODE=$(echo "$REACH_RESP" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)

if [ "$REACH_CODE" = "SUCCESS" ] || [ "$REACH_CODE" = "0" ]; then
    SELECTED_CHANNEL=$(echo "$REACH_RESP" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data',{}).get('channel',''))" 2>/dev/null)
    echo "  ✅ 3a. dry_run 选渠道成功 (code=$REACH_CODE, selected_channel=$SELECTED_CHANNEL)"
    if [ "$SELECTED_CHANNEL" = "email" ]; then
        echo "      ✅ 正确选中 email 渠道 (因为客户有 email 身份，且 dry_run 跳过账号检查)"
    fi
else
    echo "  ❌ 3a. dry_run 选渠道失败 (code=$REACH_CODE, resp=$REACH_RESP)"
    echo "      可能原因: dry_run 逻辑未正确跳过账号检查"
fi

# --------------------------------------------------
# Test 4: Bridge 参数绑定修复 (使用 Bridge 支持的渠道)
# --------------------------------------------------
echo -e "\n[Step 4] 回归测试: Bridge ingest body 参数绑定"
echo "  验证: channel/account_id 可从 JSON Body 传递 (使用 Bridge 支持的 douyin)"

# 4a. 从 Body 传参数 (不传 Query)
echo "  -> 测试 JSON Body 传参 (channel=douyin)"
BRIDGE_RESP=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"channel":"douyin","account_id":"regression_test_acc","messages":[{"from_user_id":"dy_user_001","content":"你好"}]}' \
    "$BASE/bridge/ingest")

if echo "$BRIDGE_RESP" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d.get('ok') == True" 2>/dev/null; then
    echo "  ✅ 4a. Bridge ingest (Body 传参) 成功"
else
    echo "  ❌ 4a. Bridge ingest (Body 传参) 失败: $BRIDGE_RESP"
    echo "      可能原因: handler_http.go 的 body fallback 逻辑未生效"
fi

# --------------------------------------------------
# Test 5: 清理测试数据
# --------------------------------------------------
echo -e "\n[Step 5] 清理测试数据"
ONE_ID=$(docker exec mtk-postgres psql -U admin -d user_db -p 8202 -t -A -c "SELECT unified_id FROM customers WHERE id = '$CUSTOMER_ID';" 2>/dev/null)
docker exec mtk-postgres psql -U admin -d user_db -p 8202 -c \
    "DELETE FROM customer_channels WHERE one_id = '$ONE_ID' AND channel = 'telegram';" 2>/dev/null
echo "  ✅ 已清理 telegram 绑定数据"

echo -e "\n======================================"
echo " 回归测试完成"
echo "======================================"
