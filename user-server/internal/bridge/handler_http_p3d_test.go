package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	h.ingress = svc

	const (
		channel   = "douyin_web"
		accountID = "acc_p3d_1"
		conv      = "conv_p3d"
	)
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

	if n, err := svc.AckOutboundDelivered(context.Background(), channel, accountID, []string{"mh:msg_b content"}); err != nil || n != 1 {
		t.Fatalf("首次 ack msg_b 应返回 1，实际 (%d, %v)", n, err)
	}

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
		`"affected_count":2`,     
		`"acked_items_count":2`,  
		`"duplicate_count":1`,    
		`"not_found_count":1`,    
	}
	for _, e := range expects {
		if !strings.Contains(respStr, e) {
			t.Errorf("响应缺少 %q\n实际响应: %s", e, respStr)
		}
	}
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
	res, err := svc.AckOutboundDeliveredDetailed(ctx, channel, accountID, []string{"mh:cross"}, "", "delivered", nil)
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

// TestAckOutboundDeliveredDetailed_ConcurrentDoubleAck_P4 验证 P4-2.1：并发双重 ack。
//
// 场景：N=10 个 goroutine 同时 ack 同一 msg_id，最终只能 1 个 acked + 9 个 duplicate。
// 之前"先查后更"实现会出现 acked 计数虚高；P4 修复后用 RETURNING 单 SQL 原子"分类+翻转"。
// 跑 go test -race 检测竞态。
func TestAckOutboundDeliveredDetailed_ConcurrentDoubleAck_P4(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub").Error; err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	svc := service.NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()
	const (
		channel   = "douyin_web"
		accountID = "acc_race"
		msgID     = "mh:race_msg"
	)
	hub := &model.MessageHub{
		Platform:       channel,
		AccountID:      accountID,
		ConversationID: "conv_race",
		MsgID:          msgID,
		MsgType:        "text",
		Content:        "race content",
		Direction:      "outbound",
		Status:         "pending",
	}
	if err := db.Create(hub).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 10 个 goroutine 并发 ack 同一 msg_id
	const N = 10
	results := make([]*service.AckOutboundResult, N)
	errs := make([]error, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			r, e := svc.AckOutboundDeliveredDetailed(ctx, channel, accountID, []string{msgID}, "", "delivered", nil)
			results[idx] = r
			errs[idx] = e
		}(i)
	}
	wg.Wait()
	totalAcked := 0
	totalDup := 0
	for i, r := range results {
		if errs[i] != nil {
			t.Errorf("worker %d err: %v", i, errs[i])
			continue
		}
		if r == nil {
			t.Errorf("worker %d nil result", i)
			continue
		}
		totalAcked += r.AckedItemsCount
		totalDup += r.DuplicateCount
	}
	if totalAcked != 1 {
		t.Errorf("期望总 acked_items_count=1（仅 1 个真正翻转），实际 %d", totalAcked)
	}
	if totalDup != N-1 {
		t.Errorf("期望总 duplicate=%d（其余 9 个幂等跳过），实际 %d", N-1, totalDup)
	}
}

// TestAckOutboundDeliveredDetailed_HubRepoNil_P4 验证 P4-3.1：hubRepo nil 必须返 error。
// 注：NewInboxIngressServiceWithDB 在 db!=nil 时会自动装配默认 hubRepo（8/18 起），
// 因此 nil-repo 场景通过 db=nil 构造。
func TestAckOutboundDeliveredDetailed_HubRepoNil_P4(t *testing.T) {
	svc := service.NewInboxIngressServiceWithDB(nil, nil)
	r, err := svc.AckOutboundDeliveredDetailed(context.Background(), "douyin", "acc", []string{"m1"}, "", "delivered", nil)
	if err == nil {
		t.Fatalf("hubRepo nil 应返回 error，实际 (result=%+v, err=nil)", r)
	}
	if r != nil {
		t.Errorf("hubRepo nil 应返 nil result，实际 %+v", r)
	}
}

// TestAckOutboundDeliveredDetailed_DuplicateMsgIDInput_P4 验证 P4-7.4：msg_id 入参去重。
func TestAckOutboundDeliveredDetailed_DuplicateMsgIDInput_P4(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub").Error; err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	svc := service.NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()
	hub := &model.MessageHub{
		Platform:       "douyin",
		AccountID:      "acc_dup",
		ConversationID: "c",
		MsgID:          "mh:dup",
		MsgType:        "text",
		Content:        "dup content",
		Direction:      "outbound",
		Status:         "pending",
	}
	if err := db.Create(hub).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	r, err := svc.AckOutboundDeliveredDetailed(ctx, "douyin", "acc_dup", []string{"mh:dup", "mh:dup", "mh:dup"}, "", "delivered", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(r.Items) != 1 {
		t.Errorf("期望 items=1（去重后），实际 %d", len(r.Items))
	}
	if r.AckedItemsCount != 1 {
		t.Errorf("期望 acked_items_count=1，实际 %d", r.AckedItemsCount)
	}
}

// TestGetByMsgIDsInScope_OnlyOutbound_P4 验证 P4-1.1：GetByMsgIDsInScope 仅返回 outbound 行。
func TestGetByMsgIDsInScope_OnlyOutbound_P4(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub").Error; err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	hubRepo := repository.NewMessageHubRepositoryWithDB(db)
	ctx := context.Background()
	for _, c := range []struct {
		direction, msg, conv string
	}{
		// 唯一约束 uni_message_hub_platform_msg_conv 不含 direction，
		// 同 msg_id 的 inbound/outbound 须用不同 conversation_id 才能共存。
		{"inbound", "mh:shared", "c_in"},
		{"outbound", "mh:shared", "c"},
	} {
		h := &model.MessageHub{
			Platform:       "douyin",
			AccountID:      "acc_d",
			ConversationID: c.conv,
			MsgID:          c.msg,
			MsgType:        "text",
			Content:        c.msg,
			Direction:      c.direction,
			Status:         "pending",
		}
		if err := db.Create(h).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rows, err := hubRepo.GetByMsgIDsInScope(ctx, "douyin", "acc_d", []string{"mh:shared"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	outboundCount := 0
	for _, h := range rows {
		if h.Direction == "outbound" {
			outboundCount++
		}
	}
	if outboundCount != 1 {
		t.Errorf("期望仅返回 1 条 outbound，实际 %d 条（rows=%+v）", outboundCount, rows)
	}
}

