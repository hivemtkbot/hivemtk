-- wrk POST body 脚本（/tmp/ingest.lua）
-- 配合 wrk-bench.sh 使用
wrk.method = "POST"
wrk.body = [[
{"channel":"xiaohongshu","account_id":"bench","agent_id":"a1","conversation_id":"c1","messages":[{"msg_id":"m1","event_id":"e1","sender_type":"customer","text":"hi","timestamp":1700000000000}]}
]]
wrk.headers["Content-Type"] = "application/json"
wrk.headers["Authorization"] = "Bearer test-token"
