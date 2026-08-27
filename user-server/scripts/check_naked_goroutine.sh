#!/usr/bin/env bash
#
# check_naked_goroutine.sh — CI 兜底：禁止业务代码新增裸 go func( 调用。
#
# 用法：
#   在 user-server 根目录执行：
#     ./scripts/check_naked_goroutine.sh
#   或在任意位置（脚本会自动定位到 user-server 根目录）：
#     bash /path/to/user-server/scripts/check_naked_goroutine.sh
#
# 退出码：
#   0 = 通过（无裸 go func(）
#   1 = 失败（存在裸调用）
#
# 设计动机：
#   最高标准审计发现：55/76 处 `go func` 无 recover，任一 panic 将击穿
#   gin.Recovery 并击垮整个 user-server 进程。
#   统一收口到 utils.SafeGo / SafeGoDetached / SafeGoWithRetry 三个安全入口
#   是"约束优于建议"的标准工程实践。
#
# 白名单（合理豁免）：
#   - _test.go               测试文件允许裸 goroutine
#   - recover.go             本包的 recover 实现
#   - safe.go / safego.go    同包等价命名兼容
#   - async/*.go             异步工具子包内部实现（自身就是 SafeGo 的底层）

set -euo pipefail

# 自动定位 user-server 根目录（脚本放在 user-server/scripts 下）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [[ ! -d "${ROOT_DIR}" ]]; then
  echo "❌ 无法定位 user-server 根目录: ${ROOT_DIR}"
  exit 2
fi

cd "${ROOT_DIR}"

echo "🔍 扫描 ${ROOT_DIR} 下的裸 go func( 调用 ..."

# 收集可疑匹配
# 注意：
#   1) 用 [[:space:]]go func( 要求 "go" 前只有空白（排除注释 / 字符串内文本）
#   2) 行尾注释 / 多行注释同样会被命中——这是有意的（强制人工确认）
#   3) 如确为注释，请在 go 关键字前加 //：// go func(...) → 自动排除
bad=$(grep -rEn '^[[:space:]]*go func\(' . \
    --include="*.go" \
    --exclude-dir=".git" \
    --exclude-dir="vendor" \
    --exclude-dir="node_modules" \
  | grep -v "_test\.go:" \
  | grep -v "internal/pkg/utils/recover\.go:" \
  | grep -v "internal/pkg/utils/safe\.go:" \
  | grep -v "internal/pkg/utils/safego\.go:" \
  | grep -v "internal/pkg/utils/async/" \
  | grep -v "internal/pkg/utils/backoff\.go:" \
  || true)

if [[ -n "${bad}" ]]; then
  echo ""
  echo "❌ 检测到裸 go func( 调用，应改用 utils.SafeGo / SafeGoDetached / SafeGoWithRetry："
  echo ""
  echo "${bad}"
  echo ""
  echo "迁移示例："
  echo "  // 旧"
  echo "  go func() {"
  echo "      defer recover()"
  echo "      doWork()"
  echo "  }()"
  echo ""
  echo "  // 新（推荐）"
  echo "  utils.SafeGo(ctx, \"module.action\", func(ctx context.Context) {"
  echo "      doWork(ctx)"
  echo "  })"
  echo ""
  echo "  // 长任务（与请求 ctx 解耦，带超时）"
  echo "  utils.SafeGoDetached(ctx, \"module.background\", 5*time.Minute, func(ctx context.Context) {"
  echo "      doWork(ctx)"
  echo "  })"
  echo ""
  echo "  // 带退避重试"
  echo "  b := utils.NewExponentialBackOff()"
  echo "  utils.SafeGoWithRetry(ctx, \"module.retry\", b, func(ctx context.Context) error {"
  echo "      return doWork(ctx)"
  echo "  })"
  echo ""
  exit 1
fi

echo "✅ 全部 go func( 已 SafeGo 化"
exit 0