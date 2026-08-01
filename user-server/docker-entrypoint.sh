#!/bin/sh
# ============================================================
# 容器入口脚本
# ------------------------------------------------------------
# 根因：命名卷（mtk_user_logs / mtk_user_uploads / mtk_user_data）
# 首次挂载时由 Docker 以 root 属主初始化，而应用以 app 用户运行，
# 导致 zerolog 写 /app/logs 报 "permission denied"（仅 warning 但污染日志）。
# 解决：以 root（镜像默认用户）修正挂载卷目录属主，再降级到 app 用户
# 运行主程序，避免切换 USER 后无法 chown 的问题。
# ============================================================
set -e

# 修正挂载卷目录属主（卷默认 root 属主，app 用户无写权限）
chown -R app:app /app/logs /app/uploads /app/data 2>/dev/null || true

# 依据 RUN_MODE 选择运行模式（dev=air 热更新，其余=生产二进制）
# RUN_MODE 为空（生产默认）时走生产二进制
if [ "${RUN_MODE}" = "dev" ]; then
  exec su -s /bin/sh app -c "/usr/local/bin/air"
else
  exec su -s /bin/sh app -c "/app/user-server"
fi
