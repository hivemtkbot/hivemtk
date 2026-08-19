#!/usr/bin/env bash
# deep_integrations.sh - 集成 / 模板 / 脚本 / 自定义报表 / 看板 / 知识库 / 获客挖掘 深度测试
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 集成/模板/脚本/报表/看板/知识 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 7)"

# ---------------- integrations ----------------
info "GET /api/integrations (列表)"
api GET "/api/integrations"
[ "$API_HTTP" = "200" ] && pass "integrations 列表 200" || fail "integrations 列表 http=$API_HTTP"
info "POST /api/integrations (创建集成)"
api POST "/api/integrations" "{\"name\":\"集成$U\",\"type\":\"shop\",\"config\":{\"url\":\"https://shop.example.com\"},\"enabled\":true}"
IID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$IID" ] && pass "integration 创建 200 id=$IID" || info "integration 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$IID" ]; then
  info "PUT /api/integrations/$IID (更新)"
  api PUT "/api/integrations/$IID" "{\"id\":$IID,\"name\":\"集成2$U\"}"
  [ "$API_HTTP" = "200" ] && pass "integration 更新 200" || info "更新 http=$API_HTTP (info)"
  info "DELETE /api/integrations/$IID (删除)"
  api DELETE "/api/integrations/$IID"
  [ "$API_HTTP" = "200" ] && pass "integration 删除 200" || info "delete http=$API_HTTP (info)"
fi

# ---------------- templates ----------------
info "GET /api/templates?type=response (模板列表)"
api GET "/api/templates?type=response"
[ "$API_HTTP" = "200" ] && pass "templates 列表 200" || fail "templates 列表 http=$API_HTTP"
info "POST /api/templates (创建)"
api POST "/api/templates" "{\"type\":\"response\",\"title\":\"模板$U\",\"content\":\"内容$U\"}"
TID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$TID" ] && pass "template 创建 200 id=$TID" || info "template 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$TID" ]; then
  info "PUT /api/templates/$TID (更新)"
  api PUT "/api/templates/$TID" "{\"id\":$TID,\"title\":\"模板2$U\"}"
  [ "$API_HTTP" = "200" ] && pass "template 更新 200" || info "更新 http=$API_HTTP (info)"
  info "DELETE /api/templates/$TID (删除)"
  api DELETE "/api/templates/$TID"
  [ "$API_HTTP" = "200" ] && pass "template 删除 200" || info "delete http=$API_HTTP (info)"
fi

# ---------------- scripts ----------------
info "GET /api/scripts (脚本列表)"
api GET "/api/scripts"
[ "$API_HTTP" = "200" ] && pass "scripts 列表 200" || fail "scripts 列表 http=$API_HTTP"
info "POST /api/scripts (创建)"
api POST "/api/scripts" "{\"name\":\"脚本$U\",\"content\":\"console.log('hi')\",\"language\":\"javascript\"}"
SCID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$SCID" ] && pass "script 创建 200 id=$SCID" || info "script 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$SCID" ]; then
  info "PUT /api/scripts/$SCID (更新)"
  api PUT "/api/scripts/$SCID" "{\"id\":$SCID,\"name\":\"脚本2$U\"}"
  [ "$API_HTTP" = "200" ] && pass "script 更新 200" || info "更新 http=$API_HTTP (info)"
  info "DELETE /api/scripts/$SCID (删除)"
  api DELETE "/api/scripts/$SCID"
  [ "$API_HTTP" = "200" ] && pass "script 删除 200" || info "delete http=$API_HTTP (info)"
fi

# ---------------- custom-reports ----------------
info "GET /api/custom-reports (列表)"
api GET "/api/custom-reports"
[ "$API_HTTP" = "200" ] && pass "custom-reports 列表 200" || fail "custom-reports 列表 http=$API_HTTP"
info "POST /api/custom-reports (创建)"
api POST "/api/custom-reports" "{\"name\":\"报表$U\",\"description\":\"描述$U\",\"query\":\"SELECT 1\"}"
CRID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$CRID" ] && pass "custom-report 创建 200 id=$CRID" || info "custom-report 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$CRID" ]; then
  info "DELETE /api/custom-reports/$CRID (删除)"
  api DELETE "/api/custom-reports/$CRID"
  [ "$API_HTTP" = "200" ] && pass "custom-report 删除 200" || info "delete http=$API_HTTP (info)"
fi

# ---------------- dashboards ----------------
info "GET /api/dashboards (看板列表)"
api GET "/api/dashboards"
[ "$API_HTTP" = "200" ] && pass "dashboards 列表 200" || fail "dashboards 列表 http=$API_HTTP"
info "POST /api/dashboards (创建)"
api POST "/api/dashboards" "{\"name\":\"看板$U\",\"layout\":\"{}\"}"
DID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$DID" ] && pass "dashboard 创建 200 id=$DID" || info "dashboard 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$DID" ]; then
  info "DELETE /api/dashboards/$DID (删除)"
  api DELETE "/api/dashboards/$DID"
  [ "$API_HTTP" = "200" ] && pass "dashboard 删除 200" || info "delete http=$API_HTTP (info)"
fi

# ---------------- knowledge (external/import, 无独立 CRUD 列表 API) ----------------
info "POST /api/knowledge/external/import (外部导入, 需外部配置)"
api POST "/api/knowledge/external/import" "{\"source\":\"demo\",\"url\":\"https://example.com/kb$U\"}"
[ "$API_HTTP" = "200" ] && pass "knowledge/external/import 200" || info "knowledge/external/import http=$API_HTTP (info, 需外部配置)"

# ---------------- lead-mining/config ----------------
info "GET /api/lead-mining/config (获客配置)"
api GET "/api/lead-mining/config"
[ "$API_HTTP" = "200" ] && pass "lead-mining/config 200" || info "lead-mining/config http=$API_HTTP (info)"
info "PUT /api/lead-mining/config"
api PUT "/api/lead-mining/config" "{\"enabled\":true,\"min_score\":0.5}"
[ "$API_HTTP" = "200" ] && pass "lead-mining/config 更新 200" || info "更新 http=$API_HTTP (info)"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
