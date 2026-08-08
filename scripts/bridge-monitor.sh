#!/usr/bin/env bash
#
# bridge-monitor.sh — 桥接（bridge）功能健康巡检
#
# 读取两路信号，综合判断 bridge 是否正常工作：
#   1) 上报日志：容器最近 N 分钟的 bridge 相关 API 交互日志（ingest / outbox / ack / api_interaction / error）
#   2) 数据库数据：message_hub（上行/下行队列）、bridge_accounts（扩展连接）、inbox_conversations（会话）
#
# 用法:
#   bash scripts/bridge-monitor.sh [窗口]       窗口默认 30m（如 1h, 15m）
#   BRIDGE_CONTAINER=mtk-user-server-dev bash scripts/bridge-monitor.sh
#
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/../.env"
COMPOSE_DIR="$SCRIPT_DIR/.."

# ---- 可调参数（环境变量覆盖）----
SINCE="${1:-30m}"
BRIDGE_CONTAINER="${BRIDGE_CONTAINER:-mtk-user-server}"
DB_HOST="${BRIDGE_DB_HOST:-127.0.0.1}"
DB_PORT="${BRIDGE_DB_PORT:-${USER_POSTGRES_HOST_PORT:-8232}}"
DB_USER="${BRIDGE_DB_USER:-${POSTGRES_USER:-admin}}"
DB_NAME="${BRIDGE_DB_NAME:-${USER_DB_NAME:-user_db}}"

# ---- 颜色 ----
if [ -t 1 ]; then
  C_OK=$'\033[32m'; C_WARN=$'\033[33m'; C_FAIL=$'\033[31m'; C_DIM=$'\033[2m'; C_RST=$'\033[0m'
else
  C_OK=""; C_WARN=""; C_FAIL=""; C_DIM=""; C_RST=""
fi

ok()   { echo "${C_OK}[OK]${C_RST}   $*"; }
warn() { echo "${C_WARN}[WARN]${C_RST} $*"; }
fail() { echo "${C_FAIL}[FAIL]${C_RST} $*"; }
dim()  { echo "${C_DIM}      $*${C_RST}"; }

# ---- 加载 .env（获取 DB 密码等，可选）----
if [ -f "$ENV_FILE" ]; then
  set -a
  # 仅加载存在的变量，避免覆盖已显式设置的环境变量
  while IFS='=' read -r k v; do
    k="$(echo "$k" | xargs)"; v="$(echo "$v" | xargs)"
    [ -z "$k" ] && continue
    case "$k" in \#*) continue ;; esac
    [ -n "${!k:-}" ] && continue
    export "$k=$v"
  done < "$ENV_FILE"
  set +a
fi
DB_PASSWORD="${BRIDGE_DB_PASSWORD:-${POSTGRES_PASSWORD:-}}"
DB_PORT="${BRIDGE_DB_PORT:-${USER_POSTGRES_HOST_PORT:-8232}}"
DB_NAME="${BRIDGE_DB_NAME:-${USER_DB_NAME:-user_db}}"
DB_USER="${BRIDGE_DB_USER:-${POSTGRES_USER:-admin}}"

echo "============================================================"
echo " Bridge 功能健康巡检  (窗口=${SINCE}, 容器=${BRIDGE_CONTAINER})"
echo " 时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "============================================================"

# ---------- 1) 日志信号 ----------
echo
echo "【1/2】上报日志（最近 ${SINCE}）"
LOG_LINES=""
if command -v docker >/dev/null 2>&1; then
  LOG_LINES="$(docker logs "$BRIDGE_CONTAINER" --since "$SINCE" 2>&1)"
  if [ $? -ne 0 ] || [ -z "$LOG_LINES" ]; then
    warn "无法读取容器 ${BRIDGE_CONTAINER} 日志（容器未运行或名称不符？可用 BRIDGE_CONTAINER 指定）"
    LOG_LINES=""
  fi
else
  warn "未检测到 docker，跳过日志分析"
fi

if [ -n "$LOG_LINES" ]; then
  cnt_ingest=$(printf '%s\n' "$LOG_LINES" | grep -c 'http_ingest_request' || true)
  cnt_ingest_ok=$(printf '%s\n' "$LOG_LINES" | grep -c 'http_ingest_response' || true)
  cnt_ingest_err=$(printf '%s\n' "$LOG_LINES" | grep -c 'http_ingest_failed' || true)
  cnt_api=$(printf '%s\n' "$LOG_LINES" | grep -c '"event":"api_interaction"' || true)
  # 仅统计 bridge 相关错误（避免平台端/触达工具等无关噪声），含 4xx/5xx 与 ingest 失败
  cnt_err=$(printf '%s\n' "$LOG_LINES" | grep -E '"event":"api_interaction"' | grep -E '/bridge' | grep -E '"status":[45][0-9][0-9]' | wc -l | tr -d ' ')
  cnt_err=$(( cnt_err + $(printf '%s\n' "$LOG_LINES" | grep -c 'http_ingest_failed' | tr -d ' ') ))

  dim "桥接上报(http_ingest_request): ${cnt_ingest} 次"
  dim "桥接响应(http_ingest_response): ${cnt_ingest_ok} 次"
  dim "桥接失败(http_ingest_failed):   ${cnt_ingest_err} 次"
  dim "API 交互日志(api_interaction):  ${cnt_api} 条"
  dim "桥接相关错误(4xx/5xx):          ${cnt_err} 条"

  if [ "$cnt_ingest" -gt 0 ] && [ "$cnt_ingest_err" -eq 0 ]; then
    ok "上报日志正常：桥接扩展在持续上行消息"
  elif [ "$cnt_ingest" -gt 0 ] && [ "$cnt_ingest_err" -gt 0 ]; then
    warn "上报日志存在失败：${cnt_ingest_err} 次 ingest 失败"
  elif [ "$cnt_ingest" -eq 0 ]; then
    warn "近 ${SINCE} 无桥接上报日志（可能扩展离线 / 渠道无流量 / 容器名不符）"
  fi
  if [ "$cnt_err" -gt 0 ]; then
    warn "近 ${SINCE} 检测到 ${cnt_err} 条桥接相关错误，建议查看详情"
    printf '%s\n' "$LOG_LINES" | grep -E '"event":"api_interaction".*/bridge' | grep -iE '"status":[45]' | tail -n 5 | while read -r l; do dim "$l"; done
  fi
else
  warn "无日志数据可分析"
fi

# ---------- 2) 数据库信号 ----------
echo
echo "【2/2】数据库数据"

PSQL=()
if command -v psql >/dev/null 2>&1 && [ -n "$DB_PASSWORD" ]; then
  PSQL=(psql -X -tA -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME")
  export PGPASSWORD="$DB_PASSWORD"
else
  if ! command -v psql >/dev/null 2>&1; then
    warn "未检测到 psql 客户端，跳过数据库分析"
  else
    warn "未在 .env 找到 POSTGRES_PASSWORD，跳过数据库分析"
  fi
fi

psql_val() {
  if [ ${#PSQL[@]} -eq 0 ]; then echo "N/A"; return; fi
  local out
  out="$("${PSQL[@]}" -c "$1" 2>/dev/null)"
  [ -z "$out" ] && out="0"
  echo "$out"
}

if [ ${#PSQL[@]} -gt 0 ]; then
  inbound_1h=$(psql_val "SELECT count(*) FROM message_hub WHERE direction='inbound' AND created_at > now() - interval '1 hour';")
  inbound_24h=$(psql_val "SELECT count(*) FROM message_hub WHERE direction='inbound' AND created_at > now() - interval '24 hours';")
  pending_total=$(psql_val "SELECT count(*) FROM message_hub WHERE direction='outbound' AND status='pending';")
  # 注意：EXTRACT(EPOCH ...) 返回「秒」，须 /60 换算为分钟（曾误当分钟导致阈值失真）
  pending_oldest_min=$(psql_val "SELECT COALESCE((EXTRACT(EPOCH FROM (now() - min(COALESCE(sent_at, created_at))))/60)::int, 0) FROM message_hub WHERE direction='outbound' AND status='pending';")
  # 不可达/待观察目标：
  #  - 占位账号(<channel>-unknown)：真正不可达，后端已标 failed（不会进 here，但若存量则计入）。
  #  - 昵称派生会话(conv:<名>)：前端现已尝试按列表项 name 匹配投递，可尽力送达；
  #    打不开的会留 pending，下一轮 downlink 仍可重试，归为「待观察」而非「孤儿」。
  pending_placeholder=$(psql_val "SELECT count(*) FROM message_hub WHERE direction='outbound' AND status='pending' AND account_id LIKE '%-unknown';")
  pending_convname=$(psql_val "SELECT count(*) FROM message_hub WHERE direction='outbound' AND status='pending' AND conversation_id LIKE 'conv:%';")
  # 不可达/待观察并集（UNION，非求和）：占位账号与 conv: 名有大量重叠，求和会虚高导致「可达目标」变负。
  pending_undeliverable=$(psql_val "SELECT count(*) FROM message_hub WHERE direction='outbound' AND status='pending' AND (account_id LIKE '%-unknown' OR conversation_id LIKE 'conv:%');")
  pending_deliverable=$(( pending_total - pending_undeliverable ))
  pending_oldest_deliverable_min=$(psql_val "SELECT COALESCE((EXTRACT(EPOCH FROM (now() - min(COALESCE(sent_at, created_at))))/60)::int, 0) FROM message_hub WHERE direction='outbound' AND status='pending' AND account_id NOT LIKE '%-unknown' AND conversation_id NOT LIKE 'conv:%';")
  failed_total=$(psql_val "SELECT count(*) FROM message_hub WHERE direction='outbound' AND status='failed';")
  # 卡住的 AI 回复：仅统计「可达目标」且 >10min 未送达；不可达目标本就投不出，不计入 FAIL。
  stuck_ai_deliverable=$(psql_val "SELECT count(*) FROM message_hub WHERE direction='outbound' AND status='pending' AND is_ai_reply=true AND account_id NOT LIKE '%-unknown' AND conversation_id NOT LIKE 'conv:%' AND COALESCE(sent_at, created_at) < now() - interval '10 minutes';")
  # 区分「会话在系统存在(用户未在界面打开)」vs「会话在系统无记录(可能已删/屏蔽)」：
  # 前者属小红书无法主动打开屏外会话的正常待观察(WARN)；后者才是真实投递故障(FAIL)。
  stuck_exists=$(psql_val "SELECT count(*) FROM message_hub m WHERE direction='outbound' AND status='pending' AND is_ai_reply=true AND account_id NOT LIKE '%-unknown' AND conversation_id NOT LIKE 'conv:%' AND COALESCE(sent_at, created_at) < now() - interval '10 minutes' AND EXISTS (SELECT 1 FROM inbox_conversations i WHERE i.conversation_id = m.conversation_id);")
  stuck_missing=$(psql_val "SELECT count(*) FROM message_hub m WHERE direction='outbound' AND status='pending' AND is_ai_reply=true AND account_id NOT LIKE '%-unknown' AND conversation_id NOT LIKE 'conv:%' AND COALESCE(sent_at, created_at) < now() - interval '10 minutes' AND NOT EXISTS (SELECT 1 FROM inbox_conversations i WHERE i.conversation_id = m.conversation_id);")
  acct_total=$(psql_val "SELECT count(*) FROM bridge_accounts;")
  acct_online=$(psql_val "SELECT count(*) FROM bridge_accounts WHERE status='online';")

  dim "上行消息(inbound) 近1h/近24h: ${inbound_1h} / ${inbound_24h}"
  dim "下行队列(outbound) 待发送/失败: ${pending_total} / ${failed_total}"
  dim "  其中 可达目标 / 占位账号(-unknown) / 昵称会话(conv:名): ${pending_deliverable} / ${pending_placeholder} / ${pending_convname}"
  dim "下行最旧待发送(全部/可达): ${pending_oldest_min} / ${pending_oldest_deliverable_min} 分钟"
  dim "卡住的 AI 回复(可达目标,>10min): ${stuck_ai_deliverable}（会话存在待观察:${stuck_exists} / 会话无记录故障:${stuck_missing}）"
  dim "桥接账号 总数/在线:            ${acct_total} / ${acct_online}"

  # 分渠道账号
  echo
  dim "桥接账号按渠道 (channel | 总数 | 在线):"
  if [ -n "$DB_PASSWORD" ]; then
    PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -c "SELECT channel, count(*), count(*) FILTER (WHERE status='online') FROM bridge_accounts GROUP BY channel ORDER BY channel;" 2>/dev/null | while read -r line; do dim "  $line"; done
  fi

  # 会话按平台
  echo
  dim "收件箱会话按平台 (platform | 会话数):"
  PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -c "SELECT platform, count(*) FROM inbox_conversations GROUP BY platform ORDER BY platform;" 2>/dev/null | while read -r line; do dim "  $line"; done

  # ---- 健康判定 ----
  echo
  echo "【结论】"
  overall="OK"
  if [ "$acct_total" -eq 0 ]; then
    warn "无桥接账号：扩展尚未连接注册（bridge_accounts 为空）"; overall="WARN"
  elif [ "$acct_online" -eq 0 ]; then
    warn "所有桥接账号均离线：扩展可能已全部断开"; overall="WARN"
  else
    ok "桥接账号在线：${acct_online}/${acct_total}"
  fi

  if [ "$stuck_missing" -gt 0 ]; then
    fail "有 ${stuck_missing} 条 AI 回复卡在下行队列(可达目标,且会话在系统无记录,可能已删/屏蔽)超过 10 分钟未送达——真实投递故障"; overall="FAIL"
  elif [ "$stuck_exists" -gt 0 ]; then
    warn "有 ${stuck_exists} 条 AI 回复卡在下行队列(会话在系统存在,但当前未在网页端打开;小红书无法主动打开屏外会话)——待用户打开该会话后自动投递(正常待观察)"; [ "$overall" = "OK" ] && overall="WARN"
  elif [ "$stuck_ai_deliverable" -gt 0 ]; then
    fail "有 ${stuck_ai_deliverable} 条 AI 回复卡在下行队列(可达目标)超过 10 分钟未送达（在线账号却收不到回复，属真实投递故障）"; overall="FAIL"
  elif [ "$pending_deliverable" -gt 0 ] && [ "$pending_oldest_deliverable_min" -gt 15 ]; then
    warn "下行队列有 ${pending_deliverable} 条可达目标待发送，最旧已 ${pending_oldest_deliverable_min} 分钟（xiaohongshu 等无法主动打开会话，需用户在网页端打开该会话才下发）"; [ "$overall" = "OK" ] && overall="WARN"
  elif [ "$pending_deliverable" -gt 0 ]; then
    ok "下行队列有 ${pending_deliverable} 条可达目标待发送（最旧 ${pending_oldest_deliverable_min} 分钟，正常）"
  else
    ok "下行队列无可达目标积压"
  fi

  if [ "$pending_placeholder" -gt 0 ]; then
    warn "下行队列有 ${pending_placeholder} 条 pending 属占位账号(<channel>-unknown)，真正不可达（后端已对新增标 failed）；存量建议归档"; [ "$overall" = "OK" ] && overall="WARN"
  fi
  if [ "$pending_convname" -gt 0 ]; then
    warn "下行队列有 ${pending_convname} 条 pending 属昵称派生会话(conv:<名>)：前端现会按列表项 name 尝试投递，打不开则留 pending 下一轮重试（待观察，非必失败）"; [ "$overall" = "OK" ] && overall="WARN"
  fi

  if [ "$failed_total" -gt 0 ]; then
    warn "下行队列有 ${failed_total} 条 failed 状态消息（含不可达目标标记，需排查投递失败原因）"; [ "$overall" = "OK" ] && overall="WARN"
  fi

  if [ "$inbound_1h" -eq 0 ] && [ "$inbound_24h" -gt 0 ]; then
    warn "近 1 小时无客户上行消息（可能渠道静默 / 扩展离线）"; [ "$overall" = "OK" ] && overall="WARN"
  fi

  if [ "$overall" = "OK" ]; then
    echo
    ok "Bridge 功能整体正常 ✅"
  elif [ "$overall" = "WARN" ]; then
    echo
    warn "Bridge 功能基本可用，但存在需关注项 ⚠️"
  else
    echo
    fail "Bridge 功能异常，请立即排查 ❌"
  fi
else
  warn "无数据库数据可分析"
fi

echo "============================================================"
