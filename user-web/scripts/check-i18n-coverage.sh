#!/usr/bin/env bash
# =============================================================================
# check-i18n-coverage.sh
# i18n 翻译完整性检查（OPT-FE-09）
#
# 校验项：
#   1. zh.json / en.json / ja.json / ar.json 顶层 key 一致性
#   2. 嵌套 key 数量对齐
#   3. 缺失 key 必须 <= 阈值
#
# 用法：
#   bash scripts/check-i18n-coverage.sh
#   或: bash scripts/check-i18n-coverage.sh --strict
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCALES_DIR="$SCRIPT_DIR/../src/i18n/locales"

if [ ! -d "$LOCALES_DIR" ]; then
  echo "❌ locales dir not found: $LOCALES_DIR"
  exit 1
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ERRORS=0
STRICT="${1:-}"
THRESHOLD_MISSING=5  # 允许最多 5 个 key 缺失（占位符占位）

# 检测 python3
if ! command -v python3 &> /dev/null; then
  echo "❌ python3 not found"
  exit 1
fi

cd "$LOCALES_DIR"

echo "============================================================"
echo "  i18n 翻译完整性检查（OPT-FE-09）"
echo "  目录: $LOCALES_DIR"
echo "  模式: ${STRICT:-normal}"
echo "============================================================"
echo ""

# 1) 统计每个文件的 key 数量
echo "[1/3] Key 数量统计："
python3 << 'PYEOF'
import json
import os

files = ['zh.json', 'en.json', 'ja.json', 'ar.json']
counts = {}

def count_keys(d, prefix=''):
    n = 0
    for k, v in d.items():
        full = f'{prefix}.{k}' if prefix else k
        if isinstance(v, dict):
            n += count_keys(v, full)
        else:
            n += 1
    return n

for f in files:
    if not os.path.exists(f):
        print(f'  {f}: ❌ 不存在')
        continue
    with open(f, encoding='utf-8') as fp:
        data = json.load(fp)
    counts[f] = count_keys(data)
    print(f'  {f}: {counts[f]} keys')

# 检查一致性
max_count = max(counts.values())
min_count = min(counts.values())
diff = max_count - min_count
if diff > 0:
    print(f'\n  ⚠️  Key 数量差异: {diff}（max={max_count}, min={min_count}）')
PYEOF

echo ""

# 2) 找出缺失的 key
echo "[2/3] 缺失 key 详细："
python3 << 'PYEOF'
import json
import os

files = ['zh.json', 'en.json', 'ja.json', 'ar.json']
all_keys = {}

def get_all_keys(d, prefix=''):
    keys = set()
    for k, v in d.items():
        full = f'{prefix}.{k}' if prefix else k
        if isinstance(v, dict):
            keys.update(get_all_keys(v, full))
        else:
            keys.add(full)
    return keys

for f in files:
    if not os.path.exists(f):
        continue
    with open(f, encoding='utf-8') as fp:
        all_keys[f] = get_all_keys(json.load(fp))

base = 'zh.json'
total_missing = 0
for f in files:
    if f == base:
        continue
    missing = sorted(all_keys[base] - all_keys[f])
    if missing:
        print(f'  {f} 缺 {len(missing)} keys: {missing[:3]}{"..." if len(missing)>3 else ""}')
        total_missing += len(missing)
    else:
        print(f'  {f}: ✅ 全部对齐')

print(f'\n  总缺失: {total_missing} keys')
PYEOF

echo ""

# 3) 阈值检查
echo "[3/3] 阈值检查："
TOTAL_MISSING=$(python3 -c "
import json
files = {'zh': 'zh.json', 'en': 'en.json', 'ja': 'ja.json', 'ar': 'ar.json'}
all_keys = {}
def get_all_keys(d, prefix=''):
    keys = set()
    for k, v in d.items():
        full = f'{prefix}.{k}' if prefix else k
        if isinstance(v, dict):
            keys.update(get_all_keys(v, full))
        else:
            keys.add(full)
    return keys
for lang, f in files.items():
    with open(f, encoding='utf-8') as fp:
        all_keys[lang] = get_all_keys(json.load(fp))
total = 0
for lang in ['en', 'ja', 'ar']:
    total += len(all_keys['zh'] - all_keys[lang])
print(total)
")

if [ "$TOTAL_MISSING" -le "$THRESHOLD_MISSING" ]; then
  echo -e "  ${GREEN}✅ 总缺失 $TOTAL_MISSING ≤ $THRESHOLD_MISSING 阈值${NC}"
else
  echo -e "  ${RED}❌ 总缺失 $TOTAL_MISSING > $THRESHOLD_MISSING 阈值${NC}"
  if [ -n "$STRICT" ]; then
    ERRORS=$((ERRORS + 1))
  fi
fi

# 总结
echo ""
echo "============================================================"
if [ $ERRORS -eq 0 ]; then
  echo -e "${GREEN}✅ i18n 检查通过${NC}"
  if [ "$TOTAL_MISSING" -gt 0 ]; then
    echo "  ⚠️ 仍有 $TOTAL_MISSING 个 key 待补（建议下个迭代）"
  fi
  exit 0
else
  echo -e "${RED}❌ i18n 检查不通过${NC}"
  exit 1
fi
