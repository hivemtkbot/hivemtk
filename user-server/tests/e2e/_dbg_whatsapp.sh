#!/usr/bin/env bash
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh
mtk_login
U="12345678"
# 复制 flow 的账号/草稿创建拿到 id
api POST /api/whatsapp/accounts "{\"name\":\"dbgWA$U\",\"phone_number\":\"+8613900001234\",\"api_key\":\"key_$U\"}"
WA_ID="$(jdata id)"
api POST /api/whatsapp/drafts "{\"name\":\"dbgDraft$U\",\"content\":\"内容$U\"}"
WAD_ID="$(jdata id)"
echo "WA_ID=$WA_ID WAD_ID=$WAD_ID"
BODY="{\"name\":\"dbgJob$U\",\"draft_id\":$WAD_ID,\"account_id\":$WA_ID,\"recipients\":\"+8613900001234\",\"type\":\"broadcast\"}"
echo "BODY=[$BODY]"
out=$(curl -s --max-time 20 -w '\n__HTTP__%{http_code}' -X POST \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "$BODY" "$BASE/api/whatsapp/jobs" 2>/dev/null)
echo "RAW=[$out]"
