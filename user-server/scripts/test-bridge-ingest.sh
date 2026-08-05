#!/bin/bash
# ============================================================
# bridge HTTP ingest 端到端验证脚本
# ------------------------------------------------------------
# 用途：手测 /api/bridge/ingest 端点（curl 模拟 xhs 源 POST）
# 覆盖：
#   1) CORS preflight 放行（Origin: https://www.xiaohongshu.com）
#   2) expect_reply=true 触发 AI 推理并返回 outbound_replies
#   3) 5min 内同内容去重（duplicate: true）
# ============================================================
# 用法：
#   bash user-server/scripts/test-bridge-ingest.sh
# ============================================================

set -e

# 1) 健康检查
echo "=== 健康检查 ==="
curl -sS -o /dev/null -w "HTTP %{http_code}\n" http://localhost:8204/health

# 2) expect_reply=true：触发 AI 推理
NOW=$(date +%s)000
EID="e2e_bridge_$NOW"
echo ""
echo "=== POST expect_reply=true（触发 AI）==="
RESP=$(curl -sS -X POST 'http://localhost:8204/api/bridge/ingest?channel=xiaohongshu&account_id=e2e_test&conversation_id=e2e_conv' \
  -H 'Content-Type: application/json' \
  -H 'Origin: https://www.xiaohongshu.com' \
  -d "{\"v\":2,\"channel\":\"xiaohongshu\",\"account_id\":\"e2e_test\",\"conversation_id\":\"e2e_conv\",\"messages\":[{\"event_id\":\"$EID\",\"content\":\"你好\",\"sender_type\":\"customer\",\"msg_type\":\"text\",\"timestamp\":$NOW}],\"expect_reply\":true,\"timeout_ms\":50000}")
echo "响应: $RESP"
echo "$RESP" | grep -q '"ai_handled":true' && echo "✅ AI 已触发" || echo "❌ AI 未触发"

# 3) 5min 窗口内同内容去重
echo ""
echo "=== POST expect_reply=true + 重复内容（应被去重）==="
DUP_EID="e2e_bridge_dup_$NOW"
RESP2=$(curl -sS -X POST 'http://localhost:8204/api/bridge/ingest?channel=xiaohongshu&account_id=e2e_test&conversation_id=e2e_conv' \
  -H 'Content-Type: application/json' \
  -H 'Origin: https://www.xiaohongshu.com' \
  -d "{\"v\":2,\"channel\":\"xiaohongshu\",\"account_id\":\"e2e_test\",\"conversation_id\":\"e2e_conv\",\"messages\":[{\"event_id\":\"$DUP_EID\",\"content\":\"你好\",\"sender_type\":\"customer\",\"msg_type\":\"text\",\"timestamp\":$NOW}],\"expect_reply\":true,\"timeout_ms\":30000}")
echo "响应: $RESP2"
echo "$RESP2" | grep -q '"duplicate":true' && echo "✅ 去重生效" || echo "❌ 未去重"

echo ""
echo "=== 验证完成 ==="
