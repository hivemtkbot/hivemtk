package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
)

// TestAckBridgeOutbox_DetailedItems_P3D 验证 P3-D：ack 响应包含 per-msg-id 详细状态。
//
// 场景：
//   1) seed 3 条 pending：msg_a, msg_b, msg_c
//   2) 先把 msg_b 单独 ack 一次 → msg_b 翻转为 delivered
//   3) 再批量 ack [msg_a, msg_b, msg_c, msg_d（不存在）]
//   4) 响应：acked=2（a+c）, duplicate=1（b）, not_found=1（d）
func TestAckBridgeOutbox_DetailedItems_P3D(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub").Error; err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	svc := service.NewInboxIngressServiceWithDB(db, nil)
	h := NewBridgeIngestHandlerWithMock(
		func(ctx context.Context, ev *model.MessageEvent) (*service.InboxIngressResult, error) {
			return &service.InboxIngressResult{Accepted: true}, nil
		},
		func(ctx context.Context, ev *model.MessageEvent, direction string) error { return nil },
	)
	// 注入 ingress（mock 路径走 mockHandle，但 ack 走真实 svc）
	h.ingress = svc

	const (
		channel   = "douyin_web"
		accountID = "acc_p3d_1"
		conv      = "conv_p3d"
	)
	// seed 3 条 pending
	for _, c := range []string{"msg_a content", "msg_b content", "msg_c content"} {
		hub := &model.MessageHub{
			Platform:       channel,
			AccountID:      accountID,
			ConversationID: conv,
			MsgID:          "mh:" + c,
			MsgType:        "text",
			Content:        c,
			Direction:      "outbound",
			Status:         "pending",
		}
		if err := db.Create(hub).Error; err != nil {
			t.Fatalf("seed 失败: %v", err)
		}
	}

	// 先单独 ack msg_b（让它变 delivered）
	if n, err := svc.AckOutboundDelivered(context.Background(), channel, accountID, []string{"mh:msg_b content"}); err != nil || n != 1 {
		t.Fatalf("首次 ack msg_b 应返回 1，实际 (%d, %v)", n, err)
	}

	// 构造批量 ack 请求：[a, b（已 delivered）, c, d（不存在）]
	body := `{"msg_ids":["mh:msg_a content","mh:msg_b content","mh:msg_c content","mh:msg_d content"],"status":"delivered"}`
	req := httptest.NewRequest("POST", "/api/bridge/outbox/ack?channel="+channel+"&account_id="+accountID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Request = req
	h.AckBridgeOutbox(c)
	if rr.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %s", rr.Code, rr.Body.String())
	}

	respStr := rr.Body.String()
	expects := []string{
		`"status":"ok"`,
		`"acked":2`,          // msg_a + msg_c（pending → delivered）
		`"duplicate_count":1`, // msg_b
		`"not_found_count":1`, // msg_d
	}
	for _, e := range expects {
		if !strings.Contains(respStr, e) {
			t.Errorf("响应缺少 %q\n实际响应: %s", e, respStr)
		}
	}
	// 验证每条 msg_id 的 status 字段
	perMsgChecks := []string{
		`"msg_id":"mh:msg_a content","status":"acked"`,
		`"msg_id":"mh:msg_b content","status":"duplicate"`,
		`"msg_id":"mh:msg_c content","status":"acked"`,
		`"msg_id":"mh:msg_d content","status":"not_found"`,
	}
	for _, ck := range perMsgChecks {
		if !strings.Contains(respStr, ck) {
			t.Errorf("响应缺少 per-msg-id 详情 %q\n实际响应: %s", ck, respStr)
		}
	}
}

// TestAckBridgeOutbox_TooManyMsgIDs_P3D 验证 P3-D：单次 ack msg_ids 数量上限保护。
func TestAckBridgeOutbox_TooManyMsgIDs_P3D(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub").Error; err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	svc := service.NewInboxIngressServiceWithDB(db, nil)
	h := NewBridgeIngestHandlerWithMock(nil, nil)
	h.ingress = svc

	// 构造 501 个 msg_ids
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = `mh:test`
	}
	body := `{"msg_ids":["` + strings.Join(ids, `","`) + `"],"status":"delivered"}`
	req := httptest.NewRequest("POST", "/api/bridge/outbox/ack?channel=douyin_web&account_id=acc", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Request = req
	h.AckBridgeOutbox(c)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，实际 %d", rr.Code)
	}
	body2 := rr.Body.String()
	if !strings.Contains(body2, "too many msg_ids") {
		t.Errorf("响应缺少 'too many msg_ids' 提示: %s", body2)
	}
}

// TestAckOutboundDeliveredDetailed_CrossSession_P3D 验证跨会话同 msg_id 互不影响（P3-D 详细 ack）。
func TestAckOutboundDeliveredDetailed_CrossSession_P3D(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub").Error; err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	svc := service.NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()
	const (
		channel   = "douyin_web"
		accountID = "acc_cross"
		content   = "cross session content"
	)
	// seed 同一 account 不同会话的两条 outbound
	for _, c := range []string{"conv_1", "conv_2"} {
		h := &model.MessageHub{
			Platform:       channel,
			AccountID:      accountID,
			ConversationID: c,
			MsgID:          "mh:cross",
			MsgType:        "text",
			Content:        content,
			Direction:      "outbound",
			Status:         "pending",
		}
		if err := db.Create(h).Error; err != nil {
			t.Fatalf("seed 失败: %v", err)
		}
	}
	// 详细 ack：应该两条都翻转为 delivered
	res, err := svc.AckOutboundDeliveredDetailed(ctx, channel, accountID, []string{"mh:cross"})
	if err != nil {
		t.Fatalf("AckOutboundDeliveredDetailed: %v", err)
	}
	if res.AffectedCount != 2 {
		t.Errorf("期望 affected=2（两条跨会话），实际 %d", res.AffectedCount)
	}
	if res.DuplicateCount != 0 {
		t.Errorf("期望 duplicate=0，实际 %d", res.DuplicateCount)
	}
	if res.NotFoundCount != 0 {
		t.Errorf("期望 not_found=0，实际 %d", res.NotFoundCount)
	}
	if len(res.Items) != 1 {
		t.Errorf("期望 items=1，实际 %d", len(res.Items))
	} else if res.Items[0].Status != "acked" {
		t.Errorf("期望 status=acked，实际 %s", res.Items[0].Status)
	}
}

// TestGetByMsgIDsInScope_OwnershipIsolation_P3D 验证 GetByMsgIDsInScope 严格按 (platform, account_id) 隔离。
func TestGetByMsgIDsInScope_OwnershipIsolation_P3D(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub").Error; err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	hubRepo := repository.NewMessageHubRepositoryWithDB(db)
	ctx := context.Background()
	// seed：accountA 有 msgA，accountB 有 msgB
	for _, c := range []struct {
		acc, msg string
	}{
		{"acc_A", "mh:msgA"},
		{"acc_B", "mh:msgB"},
	} {
		h := &model.MessageHub{
			Platform:       "douyin_web",
			AccountID:      c.acc,
			ConversationID: "conv",
			MsgID:          c.msg,
			MsgType:        "text",
			Content:        c.msg,
			Direction:      "outbound",
			Status:         "pending",
		}
		if err := db.Create(h).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// 查 accountA 下 [msgA, msgB]：只应返回 msgA
	rows, err := hubRepo.GetByMsgIDsInScope(ctx, "douyin_web", "acc_A", []string{"mh:msgA", "mh:msgB"})
	if err != nil {
		t.Fatalf("GetByMsgIDsInScope: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("期望 1 条（只 accountA 的 msgA），实际 %d", len(rows))
	}
	if len(rows) > 0 && rows[0].MsgID != "mh:msgA" {
		t.Errorf("期望 msgA，实际 %s", rows[0].MsgID)
	}
}
