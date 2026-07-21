#!/bin/sh
# wait-for-db.sh
# 等待 PostgreSQL 数据库准备就绪

set -e

host="$DB_HOST"
port="$DB_PORT"
user="$DB_USER"
dbname="$DB_NAME"
cmd="$@"

# 等待 PostgreSQL 数据库准备就绪
# 使用 pg_isready 检测数据库连接状态
until pg_isready -h "$host" -p "$port" -U "$user" -d "$dbname" > /dev/null 2>&1; do
  >&2 echo "PostgreSQL is unavailable - sleeping"
  sleep 2
done

>&2 echo "PostgreSQL is up - executing command: $cmd"
# 执行命令并将输出重定向，以便查看错误
$cmd
exit_code=$?
>&2 echo "Command exited with code: $exit_code"
exit $exit_code
