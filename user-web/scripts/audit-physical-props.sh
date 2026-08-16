#!/usr/bin/env bash
# USR-I18N-01: 物理属性审计脚本
# 扫描 src/ 下所有 .vue / .scss / .css 文件中的物理属性。
set -euo pipefail
cd "$(dirname "$0")/.."

# 物理属性模式（margin-left, padding-right, text-align: left/right, float: left/right, left/right: ...）
PATTERN='(margin-left|margin-right|padding-left|padding-right|text-align:\s*(left|right)|float:\s*(left|right)|(^|\s|;)\s*(left|right):\s*[0-9])'

# 排除 Element Plus 内部、已迁移文件、node_modules
EXCLUDES=(
  "--exclude-dir=node_modules"
  "--exclude-dir=dist"
  "--exclude-dir=.git"
  "--exclude=*.lock"
)

# 物理属性行数统计
echo "=== 物理属性审计 ==="
echo ""

# 1. Vue 文件
VUE_COUNT=$(grep -rEn "${PATTERN}" "${EXCLUDES[@]}" --include='*.vue' src/ 2>/dev/null | wc -l | tr -d ' ')
echo "📄 Vue 文件物理属性行数: ${VUE_COUNT}"

# 2. SCSS/CSS 文件
CSS_COUNT=$(grep -rEn "${PATTERN}" "${EXCLUDES[@]}" --include='*.scss' --include='*.css' src/ 2>/dev/null | wc -l | tr -d ' ')
echo "🎨 SCSS/CSS 物理属性行数: ${CSS_COUNT}"

# 3. JS/TS 文件（理论上不应该有，但保险起见）
JS_COUNT=$(grep -rEn "${PATTERN}" "${EXCLUDES[@]}" --include='*.js' --include='*.ts' src/ 2>/dev/null | wc -l | tr -d ' ')
echo "🔧 JS/TS 物理属性行数: ${JS_COUNT}"

TOTAL=$((VUE_COUNT + CSS_COUNT + JS_COUNT))
echo ""
echo "总计: ${TOTAL} 行（目标: 逐步减少到 0，Element Plus 内部除外）"

# 阈值检查（按需调整）
THRESHOLD="${PHYSICAL_PROP_THRESHOLD:-200}"
if [[ "${TOTAL}" -gt "${THRESHOLD}" ]]; then
  echo "❌ 物理属性超过阈值 ${THRESHOLD}，请改造后再提交"
  exit 1
fi

echo "✅ 物理属性在阈值内"
