#!/bin/bash
# 一键修复 ctx 重复添加的问题
# 处理场景：
#   1. method(ctx, context.Context, ...) -> method(context.Context, ...)
#   2. method(ctx, ctx, ...) -> method(ctx, ...)

set -e
cd /Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server

# 修复 (ctx, context.Context, ...) -> (context.Context, ...)
find internal -name "*.go" -exec sed -i '' 's/(ctx, context\.Context,/(context.Context,/g' {} \;

# 修复 (ctx, ctx, ...) -> (ctx, ...) - 谨慎使用
# 这个会有副作用，先不动

echo "Done. Now run go build to check."
