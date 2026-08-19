#!/usr/bin/env bash
# deep_all.sh - 深度回归测试总入口
# 运行 tests/e2e 下所有 deep_*.sh 模块测试（排除 deep_lib.sh / deep_smoke*），
# 汇总每个模块的 通过/失败，任一失败则整体退出码非 0，可用于 CI 回归门禁。
set +u
cd "$(dirname "$0")"

AGG_DIR="$(mktemp -d)"
TOTAL_PASS=0
TOTAL_FAIL=0
FAILED_MODULES=()

echo "════════════════════════════════════════════════════════════"
echo "  HIVEMTK 深度 API 回归测试 (真实调用 + 直连 PG 校验)"
echo "  时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo "════════════════════════════════════════════════════════════"

for f in deep_*.sh; do
  case "$f" in
    deep_lib.sh|deep_all.sh|deep_smoke*) continue;;
  esac
  [ -x "$f" ] || chmod +x "$f"
  chmod +x "$f" 2>/dev/null
  LOG="$AGG_DIR/$f.log"
  printf '%-32s ... ' "$f"
  if bash "$f" > "$LOG" 2>&1; then
    RC=0
  else
    RC=$?
  fi
  P=$(grep -c '\[PASS\]' "$LOG" || true)
  F=$(grep -c '\[FAIL\]' "$LOG" || true)
  TOTAL_PASS=$((TOTAL_PASS+P))
  TOTAL_FAIL=$((TOTAL_FAIL+F))
  if [ "$RC" -eq 0 ]; then
    echo "GREEN  (PASS=$P FAIL=$F)"
  else
    echo "RED    (PASS=$P FAIL=$F)  -> $LOG"
    FAILED_MODULES+=("$f")
  fi
done

echo "────────────────────────────────────────────────────────────"
echo "汇总: 通过 $TOTAL_PASS  失败 $TOTAL_FAIL"
if [ "${#FAILED_MODULES[@]}" -gt 0 ]; then
  echo "失败模块: ${FAILED_MODULES[*]}"
  echo "RESULT: RED"
  exit 1
fi
echo "RESULT: GREEN"
exit 0
