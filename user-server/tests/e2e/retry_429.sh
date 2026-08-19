#!/usr/bin/env bash
# 对探测结果中被限流(429)的端点加延迟重试, 确认真实响应(排除被限流掩盖的 5xx)。
typeset -x PATH="/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
E2E_DIR="$(cd "$(dirname "$0")" && pwd)"
UHOST="http://127.0.0.1:8204"
PHOST="http://127.0.0.1:8205"
TMP="$(mktemp -d)"

UPASS="Admin@123456"
UTOK=$(curl -s --max-time 10 -X POST "$UHOST/api/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$UPASS\"}" | grep -oE '"token":"[^"]+"' | head -1 | sed 's/"token":"//;s/"$//')
PPW=$(grep '^PLATFORM_ADMIN_PASSWORD=' hivemtk-platform/.env 2>/dev/null | sed 's/^PLATFORM_ADMIN_PASSWORD=//')
PTOK=$(curl -s --max-time 10 -X POST "$PHOST/api/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$PPW\"}" | grep -oE '"token":"[^"]+"' | head -1 | sed 's/"token":"//;s/"$//')

subst() { local p="$1"; p="${p//:id/1}"; p="${p//:name/test}"; p="${p//:asset_id/1}"; p="${p//:account_id/1}"; p="${p//:conversation_id/1}"; p="${p//:key/test}"; p="${p//:provider/test}"; p="${p//:sid/test}"; p="${p//:customer_id/1}"; p=$(printf '%s' "$p" | sed -E 's/:[a-zA-Z_][a-zA-Z0-9_]*/1/g'); echo "$p"; }

FAIL_REPORT="$TMP/fails.txt"
: > "$FAIL_REPORT"
cnt=0
while IFS=$'\t' read -r verdict label method path code; do
  [ "$code" = "429" ] || continue
  cnt=$((cnt+1))
  if [ "$label" = "user" ]; then host="$UHOST"; case "$path" in /api/auth/*|/health|/readyz|/chat/public/*|/share/*|/chat/embed*|/api/sso/*|/api/platform/*|/api/webhook/*|/api/bridge/*|/api/monitor/*) tok="";; *) tok="$UTOK";; esac
  else host="$PHOST"; case "$path" in /api/auth/login|/public/*|/merchant-api/*|/contributor-api/*|/platform/heartbeat|/platform/install) tok="";; /platform/*|/api/platform/*) tok="$PTOK";; /api/*) tok="$UTOK";; *) tok="$PTOK";; esac
  fi
  url="$host$(subst "$path")"
  real="429"
  for try in 1 2 3; do
    sleep 2
    if [ "$method" = "GET" ] || [ "$method" = "HEAD" ] || [ "$method" = "OPTIONS" ]; then
      if [ -n "$tok" ]; then c=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 -X "$method" -H "Authorization: Bearer $tok" "$url" 2>/dev/null); else c=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 -X "$method" "$url" 2>/dev/null); fi
    else
      if [ -n "$tok" ]; then c=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 -X "$method" -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' -d '{}' "$url" 2>/dev/null); else c=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 -X "$method" -H 'Content-Type: application/json' -d '{}' "$url" 2>/dev/null); fi
    fi
    [ -z "$c" ] && c="000"
    if [ "$c" != "429" ]; then real="$c"; break; fi
  done
  verdict2="OK"
  case "$real" in 000|500|502|503|504) verdict2="FAIL";; esac
  printf '%s\t%s\t%s\t%s\t%s\n' "$verdict2" "$label" "$method" "$path" "$real" >> "$FAIL_REPORT"
done < "$E2E_DIR/probe_result.tsv"

echo "重试 429 端点总数: $cnt"
echo "重试后仍失败(5xx/000): $(awk -F'\t' '$1=="FAIL"' "$FAIL_REPORT" | wc -l | tr -d ' ')"
awk -F'\t' '$1=="FAIL"' "$FAIL_REPORT"
echo "=== 重试后真实状态码分布(仅原 429 端点) ==="
awk -F'\t' '{print $5}' "$FAIL_REPORT" | sort | uniq -c | sort -rn
rm -rf "$TMP"
