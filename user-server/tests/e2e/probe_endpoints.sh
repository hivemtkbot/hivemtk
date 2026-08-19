#!/usr/bin/env bash
# 端到端冒烟探测: 真实 curl 两端所有已注册端点。
# 判据: 5xx / 000(连接失败/超时/panic) = FAIL; 其余(2xx/3xx/4xx) = 端点存活正常。
typeset -x PATH="/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

UHOST="http://127.0.0.1:8204"
PHOST="http://127.0.0.1:8205"
TIMEOUT=10
E2E_DIR="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"

echo "==> 登录 user-server(admin)"
UPASS="Admin@123456"
ULOGIN=$(curl -s --max-time $TIMEOUT -X POST "$UHOST/api/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$UPASS\"}")
UTOK=$(printf '%s' "$ULOGIN" | grep -oE '"token":"[^"]+"' | head -1 | sed 's/"token":"//;s/"$//')
if [ -z "$UTOK" ]; then echo "!! user-server 登录失败: $ULOGIN"; UTOK=""; fi
echo "    UTOK=${UTOK:0:24}..."

echo "==> 登录 platform-server(admin)"
PPW=$(grep '^PLATFORM_ADMIN_PASSWORD=' hivemtk-platform/.env 2>/dev/null | sed 's/^PLATFORM_ADMIN_PASSWORD=//')
PLOGIN=$(curl -s --max-time $TIMEOUT -X POST "$PHOST/api/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$PPW\"}")
PTOK=$(printf '%s' "$PLOGIN" | grep -oE '"token":"[^"]+"' | head -1 | sed 's/"token":"//;s/"$//')
if [ -z "$PTOK" ]; then echo "!! platform-server 登录失败: $PLOGIN"; PTOK=""; fi
echo "    PTOK=${PTOK:0:24}..."

# 参数化路径占位
subst() {
  local p="$1"
  p="${p//:id/1}"
  p="${p//:name/test}"
  p="${p//:asset_id/1}"
  p="${p//:account_id/1}"
  p="${p//:conversation_id/1}"
  p="${p//:key/test}"
  p="${p//:provider/test}"
  p="${p//:sid/test}"
  p="${p//:customer_id/1}"
  p="${p//:id_test/test}"
  # 兜底其它 :param
  p=$(printf '%s' "$p" | sed -E 's/:\[a-zA-Z_][a-zA-Z0-9_]*/1/g')
  echo "$p"
}

# 探测单个端点; 参数为: HOST TOKEN METHOD PATH LABEL
probe() {
  local host="$1" tok="$2" method="$3" path="$4" label="$5"
  local url="$host$(subst "$path")"
  local code body
  if [ "$method" = "GET" ] || [ "$method" = "HEAD" ] || [ "$method" = "OPTIONS" ]; then
    if [ -n "$tok" ]; then
      body=$(curl -s --max-time "$TIMEOUT" -w '\n__CODE__%{http_code}' -X "$method" -H "Authorization: Bearer $tok" "$url" 2>/dev/null)
    else
      body=$(curl -s --max-time "$TIMEOUT" -w '\n__CODE__%{http_code}' -X "$method" "$url" 2>/dev/null)
    fi
  else
    if [ -n "$tok" ]; then
      body=$(curl -s --max-time "$TIMEOUT" -w '\n__CODE__%{http_code}' -X "$method" -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' -d '{}' "$url" 2>/dev/null)
    else
      body=$(curl -s --max-time "$TIMEOUT" -w '\n__CODE__%{http_code}' -X "$method" -H 'Content-Type: application/json' -d '{}' "$url" 2>/dev/null)
    fi
  fi
  code=$(printf '%s' "$body" | sed -n 's/.*__CODE__//p')
  if [ -z "$code" ]; then code="000"; fi
  local verdict="OK"
  # 真实故障: 连接失败/网关错误
  if [ "$code" = "000" ] || [ "$code" = "500" ] || [ "$code" = "502" ] || [ "$code" = "503" ] || [ "$code" = "504" ]; then
    verdict="FAIL"
  fi
  printf '%s\t%s\t%s\t%s\t%s\n' "$verdict" "$label" "$method" "$path" "$code"
}

SUMMARY="$TMP/summary.txt"
: > "$SUMMARY"
FAIL_U="$TMP/fail_user.txt"
FAIL_P="$TMP/fail_platform.txt"

echo "==> 探测 user-server 端点"
while IFS=$'\t' read -r label method path; do
  [ "$label" = "user" ] || continue
  # 选择 token: 公开端点不带 token, 其余带 UTOK
  case "$path" in
    /api/auth/login|/api/auth/*|/health|/healthz|/readyz|/chat/public/*|/share/*|/chat/embed*|/api/sso/*|/api/platform/*|/api/app-config/health|/api/webhook/*|/api/bridge/*|/api/monitor/*)
      tok="" ;;
    *)
      tok="$UTOK" ;;
  esac
  res=$(probe "$UHOST" "$tok" "$method" "$path" user)
  echo "$res" >> "$SUMMARY"
  printf '%s\n' "$res" | awk -F'\t' '$1=="FAIL"{print}' >> "$FAIL_U"
done < "$E2E_DIR/routes_user.tsv"

echo "==> 探测 platform-server 端点"
while IFS=$'\t' read -r label method path; do
  [ "$label" = "platform" ] || continue
  case "$path" in
    /api/auth/login|/public/*|/merchant-api/*|/contributor-api/*|/platform/heartbeat|/platform/install)
      tok="" ;;
    /platform/*|/api/platform/*)
      tok="$PTOK" ;;
    /api/*)
      tok="$UTOK" ;;   # 平台 user 体系, 大概率 401(存活)
    *)
      tok="$PTOK" ;;
  esac
  res=$(probe "$PHOST" "$tok" "$method" "$path" platform)
  echo "$res" >> "$SUMMARY"
  printf '%s\n' "$res" | awk -F'\t' '$1=="FAIL"{print}' >> "$FAIL_P"
done < "$E2E_DIR/routes_platform.tsv"

echo
echo "================ 结果汇总 ================"
TOTAL=$(wc -l < "$SUMMARY")
OKN=$(awk -F'\t' '$1=="OK"' "$SUMMARY" | wc -l | tr -d ' ')
FAILN=$(awk -F'\t' '$1=="FAIL"' "$SUMMARY" | wc -l | tr -d ' ')
echo "总端点: $TOTAL | 存活(OK): $OKN | 失败(FAIL): $FAILN"
echo
if [ "$FAILN" -gt 0 ]; then
  echo "---------------- 失败端点 (5xx/000) ----------------"
  echo "--- user-server ---"; cat "$FAIL_U" 2>/dev/null
  echo "--- platform-server ---"; cat "$FAIL_P" 2>/dev/null
fi
echo "=========================================="

# 写出完整报告
cp "$SUMMARY" "$E2E_DIR/probe_result.tsv"
echo "完整结果: $E2E_DIR/probe_result.tsv"
rm -rf "$TMP"
