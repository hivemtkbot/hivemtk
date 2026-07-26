#!/bin/bash
# check-ctx-missing.sh - 统计 repository 包中未接收 context 的方法
#
#   - 扫描 internal/repository/*.go（排除 _test.go）
#   - 提取所有以 (r *xxx) 开头的方法签名
#   - 检查参数列表是否包含 ctx context.Context
#   - 按文件统计缺失 ctx 的方法数
#   - 输出汇总 + Top 10 缺 ctx 最多的文件

REPO_DIR="${1:-internal/repository}"
cd "$(dirname "$0")/.."

echo "==== Repository ctx 缺失检测 ===="
echo "扫描目录：$REPO_DIR"
echo ""

total_missing=0
total_methods=0

declare -a file_missing_counts

for f in "$REPO_DIR"/*.go; do
    [[ "$f" == *_test.go ]] && continue
    [[ ! -f "$f" ]] && continue

    # 提取所有方法签名（func (r *xxx) MethodName(...) ...）
    # 使用 awk 提取方法签名，识别是否含 context.Context 参数
    file_missing=$(awk '
        /func \(r \*[A-Za-z_]+\) [A-Z]/ {
            # 收集方法签名直到右括号
            line = $0
            while (line !~ /\)/ && getline next_line) {
                line = line " " next_line
            }

            # 跳过 GetDB 这种特殊方法
            if (line ~ /GetDB\(/ || line ~ /WithTx\(/ || line ~ /SetDB\(/ || line ~ /DB\(\)/) {
                next
            }

            # 提取方法名
            match(line, /func \(r \*[A-Za-z_]+\) ([A-Z][A-Za-z0-9_]*)/, arr)
            method_name = arr[1]
            if (method_name == "") next

            # 检查是否包含 ctx context.Context
            if (line !~ /ctx[ \t]+context\.Context/ && line !~ /context\.Context/) {
                print FILENAME ":" method_name
            }
        }
    ' "$f" 2>/dev/null)

    if [[ -n "$file_missing" ]]; then
        count=$(echo "$file_missing" | wc -l | tr -d ' ')
        total_missing=$((total_missing + count))
        file_missing_counts+=("$count $f")
        echo "❌ $f: $count 个方法缺 ctx"
        echo "$file_missing" | head -5
        if [[ "$count" -gt 5 ]]; then
            echo "   ... 还有 $((count - 5)) 个"
        fi
        echo ""
    fi
done

echo "==== 汇总 ===="
echo "缺 ctx 的方法总数：$total_missing"

# 排序
echo ""
echo "==== Top 10 缺 ctx 最多的文件 ===="
printf '%s\n' "${file_missing_counts[@]}" | sort -rn | head -10
