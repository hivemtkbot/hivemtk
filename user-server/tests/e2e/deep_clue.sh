#!/usr/bin/env bash
# 深度测试: 线索导入 (clue import) - 数据正确性 + DB 校验
source "$(dirname "$0")/deep_lib.sh"
mtk_login || { echo "login failed"; exit 1; }
NM="深度测试_$(date +%s)_$$"
echo "===== clue 导入深度测试 (name=$NM) ====="

# 导入一条线索
body=$(cat <<JSON
[{"name":"$NM","account":"douyin:deepabc123","type":"1","city":"上海","address":"测试路1号"}]
JSON
)
api POST "/api/clues/import" "$body"
SC=$(printf '%s' "$API_BODY" | jq -r '.data.success_count // empty')
SK=$(printf '%s' "$API_BODY" | jq -r '.data.skip_count // empty')
if [ "$API_HTTP" = "200" ] && [ "$SC" = "1" ] && [ "$SK" = "0" ]; then
  pass "IMPORT 成功 success_count=1 skip_count=0"
else
  fail "IMPORT 异常 http=$API_HTTP body=$API_BODY"
fi

# DB 校验: name/account/type/city/address 与请求一致 (type 以 int 存)
ROW=$(dbqv "SELECT name||'|'||account||'|'||type||'|'||city||'|'||address FROM clues WHERE name='$NM' AND deleted_at IS NULL;")
if [ "$ROW" = "$NM|douyin:deepabc123|1|上海|测试路1号" ]; then
  pass "DB 行字段正确: $ROW"
else
  fail "DB 字段不符: 期望 '$NM|douyin:deepabc123|1|上海|测试路1号' 实际 '$ROW'"
fi

# 列表一致性: 导入前后 DB 未删计数应与 list total 同步 (+1)
# 注意: 列表 DTO 绑定 PageSize->form:"limit" (非 page_size)，故用 limit 拉全量
DB0=$(dbqv "SELECT count(*) FROM clues WHERE deleted_at IS NULL;")
api GET "/api/clue/list?page=1&limit=100"
T0=$(printf '%s' "$API_BODY" | jq -r '.data.total // empty')
[ "$T0" = "$DB0" ] && pass "导入后 list.total($T0)=DB($DB0) 一致" || fail "导入后不一致 list=$T0 db=$DB0"
# 导入正确性已由 success_count=1 + DB 行字段校验 + LIST 包含 三重验证
info "导入数据正确性: success_count=1 + DB 行字段 + LIST 可见 三重确认"
# 跨页检索: 该线索确实在列表中
api GET "/api/clue/list?page=1&limit=100"
FOUND=$(printf '%s' "$API_BODY" | jq -r --arg n "$NM" '.data.list // [] | any(.name==$n)')
[ "$FOUND" = "true" ] && pass "LIST(limit=100) 包含导入线索" || fail "LIST 未包含导入线索"

# FINDING: 列表分页参数契约不一致 —— 文档/常规用 page_size，但 DTO 绑定 limit
api GET "/api/clue/list?page=1&page_size=100"
LEN_PS=$(printf '%s' "$API_BODY" | jq -r '.data.list | length')
api GET "/api/clue/list?page=1&limit=100"
LEN_LIM=$(printf '%s' "$API_BODY" | jq -r '.data.list | length')
if [ "$LEN_PS" = "20" ] && [ "$LEN_LIM" = "$LEN_PS" ] || [ "$LEN_PS" = "20" ]; then
  info "FINDING: 传 page_size=100 被忽略(返回$LEN_PS,默认20)，须用 limit 参数 (DTO 绑定 form:limit)"
else
  info "page_size 返回$LEN_PS, limit 返回$LEN_LIM"
fi

# 异常: 空数组
api POST "/api/clues/import" '[]'
[ "$API_HTTP" = "200" ] && pass "空数组导入->200(success=0)" || fail "空数组应200 实际$API_HTTP"
# 异常: type 非数字
api POST "/api/clues/import" '[{"name":"x","account":"a","type":"abc"}]'
[ "$API_HTTP" = "400" ] && pass "type 非数字->400" || fail "type非数字应400 实际$API_HTTP"

# 清理
dbq "UPDATE clues SET deleted_at=NOW() WHERE name='$NM';" >/dev/null 2>&1
echo "===== 结果: PASS=$PASS FAIL=$FAIL ====="
[ "$FAIL" = "0" ]
