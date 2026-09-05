package monitor

import (
	"context"
	"os"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tracing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func readEnvPassword() string {
	roots := []string{"../.env", "../../.env", "../../../.env"}
	for _, p := range roots {
		if b, err := os.ReadFile(p); err == nil {
			for _, line := range splitLines(string(b)) {
				if len(line) > 0 && line[0] == '#' {
					continue
				}
				var k, v string
				for i := 0; i < len(line); i++ {
					if line[i] == '=' {
						k, v = line[:i], line[i+1:]
						break
					}
				}
				if k == "POSTGRES_PASSWORD" {
					return v
				}
			}
		}
	}
	return ""
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func openTestDB(t *testing.T) *gorm.DB {
	pass := envOr("POSTGRES_TEST_PASSWORD", readEnvPassword())
	dsn := "host=" + envOr("POSTGRES_TEST_HOST", "127.0.0.1") +
		" port=" + envOr("POSTGRES_TEST_PORT", "8232") +
		" user=" + envOr("POSTGRES_TEST_USER", "admin") +
		" password=" + pass +
		" dbname=" + envOr("POSTGRES_TEST_DBNAME", "user_db") +
		" sslmode=disable"
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Logf("skip: 无法连接测试库: %v", err)
		return nil
	}
	if err := gdb.AutoMigrate(&model.MessageHub{}, &model.MessageTrace{}); err != nil {
		t.Logf("skip: 迁移失败: %v", err)
		return nil
	}
	return gdb
}

func TestTracingAndMonitoring(t *testing.T) {
	gdb := openTestDB(t)
	if gdb == nil {
		t.Skip("无可用测试库，跳过集成测试")
	}
	db.SetTestDB(gdb)
	ctx := context.Background()
	conv := "conv-monitor-test-" + time.Now().Format("150405")

	inboundTrace := tracing.GenerateTraceID()
	inHub := &model.MessageHub{
		ConversationID: conv, AccountID: "acct-test", Platform: "xiaohongshu",
		Direction: "inbound", Status: "received", MsgID: "m-in-" + conv,
		TraceID: inboundTrace, Content: "hello",
	}
	if err := gdb.WithContext(ctx).Create(inHub).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	tracing.RecordNode(ctx, tracing.NodeSpan{
		TraceID: inboundTrace, ConversationID: conv, AccountID: "acct-test",
		Channel: "xiaohongshu", Node: tracing.NodeIngest, Direction: "inbound",
		MsgID: inHub.MsgID, Expected: "客户消息落库", Status: tracing.StatusOk,
	})
	tracing.RecordNode(ctx, tracing.NodeSpan{
		TraceID: inboundTrace, ConversationID: conv, AccountID: "acct-test",
		Channel: "xiaohongshu", Node: tracing.NodeInboxSync, Direction: "inbound",
		MsgID: inHub.MsgID, Expected: "inbox_conversations 同步", Status: tracing.StatusOk,
	})

	linked := tracing.LinkOutboundTraceID(ctx, conv)
	if linked != inboundTrace {
		t.Fatalf("LinkOutboundTraceID 期望复用 %q，实际 %q", inboundTrace, linked)
	}

	tracing.RecordNode(ctx, tracing.NodeSpan{
		TraceID: linked, ConversationID: conv, AccountID: "acct-test",
		Channel: "xiaohongshu", Node: tracing.NodeOutboundEnqueue, Direction: "outbound",
		MsgID: "m-out-" + conv, Input: map[string]any{"content_len": 5},
		Output:   map[string]any{"status": "pending"},
		Expected: "AI 回复落库 outbox(pending)", Status: tracing.StatusOk,
	})
	tracing.RecordNode(ctx, tracing.NodeSpan{
		TraceID: linked, ConversationID: conv, AccountID: "acct-test",
		Channel: "xiaohongshu", Node: tracing.NodeDeliveredAck, Direction: "outbound",
		MsgID: "m-out-" + conv, Output: map[string]any{"status": "delivered"},
		DurationMs: 42, Expected: "pending→delivered", Status: tracing.StatusOk,
	})

	ov, err := HealthOverview(ctx)
	if err != nil {
		t.Fatalf("HealthOverview: %v", err)
	}
	if ov.TotalTraces < 4 {
		t.Fatalf("TotalTraces 期望 >=4，实际 %d", ov.TotalTraces)
	}

	lcs, err := Lifecycle(ctx, conv, "", 5)
	if err != nil {
		t.Fatalf("Lifecycle: %v", err)
	}
	if len(lcs) == 0 {
		t.Fatalf("Lifecycle 应至少 1 轮，实际 %d", len(lcs))
	}
	first := lcs[0]
	if len(first.Nodes) < 4 {
		t.Fatalf("该轮节点数期望 >=4，实际 %d", len(first.Nodes))
	}
	if first.EndToEndMs == nil {
		t.Fatalf("应计算出端到端时延")
	}

	traces, err := Traces(ctx, 50)
	if err != nil {
		t.Fatalf("Traces: %v", err)
	}
	found := false
	for _, tr := range traces {
		if tr.ConversationID == conv {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Traces 应包含本会话 %q", conv)
	}

	nh, err := NodeHealthByChannel(ctx)
	if err != nil {
		t.Fatalf("NodeHealthByChannel: %v", err)
	}
	hit := false
	for _, n := range nh {
		if n.Channel == "xiaohongshu" && n.Total > 0 {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatalf("NodeHealthByChannel 应包含 xiaohongshu 的节点聚合")
	}

	deleted, err := PurgeOld(ctx, 100*365*24*time.Hour)
	if err != nil {
		t.Fatalf("PurgeOld: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("PurgeOld 不应删除近期数据，实际删除 %d", deleted)
	}

	gdb.WithContext(ctx).Where("conversation_id = ?", conv).Delete(&model.MessageTrace{})
	gdb.WithContext(ctx).Where("conversation_id = ?", conv).Delete(&model.MessageHub{})
}

func TestGenerateTraceID(t *testing.T) {
	a := tracing.GenerateTraceID()
	b := tracing.GenerateTraceID()
	if a == b || len(a) < 20 {
		t.Fatalf("GenerateTraceID 异常: %q / %q", a, b)
	}
}
