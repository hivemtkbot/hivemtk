package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

// T3 验收①：statuses 回执按 wamid 命中出站行，delivered/read/failed 正确翻转。
func TestDispatchWhatsAppStatuses_AllStates(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := &WebhookService{db: db}
	ctx := context.Background()

	const account = "wa-acc-t3"
	mkRow := func(msgID string) *model.MessageHub {
		row := &model.MessageHub{MsgID: msgID, Platform: "whatsapp", AccountID: account, Direction: "outbound", MsgType: "text", ConversationID: "c", Content: "x", SentAt: time.Now()}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
		return row
	}
	sent := mkRow("wamid.SENT1")
	delivered := mkRow("wamid.DELIV1")
	read := mkRow("wamid.READ1")
	failed := mkRow("wamid.FAIL1")

	statusPayload := func(entries string) []byte {
		return []byte(`{"object":"whatsapp_business_account","entry":[{"id":"WABA","changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"PNID"},"statuses":[` + entries + `]}}]}]}`)
	}

	handled, err := svc.dispatchWhatsAppStatuses(ctx, account, statusPayload(`{"id":"wamid.SENT1","status":"sent","timestamp":"1700000000","recipient_id":"8613800000000"}`))
	if err != nil || !handled {
		t.Fatalf("sent handled=%v err=%v", handled, err)
	}
	handled, _ = svc.dispatchWhatsAppStatuses(ctx, account, statusPayload(`{"id":"wamid.DELIV1","status":"delivered","timestamp":"1700000001","recipient_id":"8613800000000"}`))
	if !handled {
		t.Fatal("delivered should be handled")
	}
	handled, _ = svc.dispatchWhatsAppStatuses(ctx, account, statusPayload(`{"id":"wamid.READ1","status":"read","timestamp":"1700000002","recipient_id":"8613800000000"}`))
	if !handled {
		t.Fatal("read should be handled")
	}
	handled, _ = svc.dispatchWhatsAppStatuses(ctx, account, statusPayload(`{"id":"wamid.FAIL1","status":"failed","timestamp":"1700000003","recipient_id":"8613800000000","errors":[{"code":131047,"title":"Re-engagement message","message":"More than 24 hours have passed"}]}`))
	if !handled {
		t.Fatal("failed should be handled")
	}

	repo := repository.NewMessageHubRepositoryWithDB(db)
	gotSent, _ := repo.GetByID(ctx, sent.ID)
	if gotSent.Status != "sent" {
		t.Fatalf("sent 行应置 sent, got %s", gotSent.Status)
	}
	gotDeliv, _ := repo.GetByID(ctx, delivered.ID)
	if gotDeliv.Status != "delivered" {
		t.Fatalf("delivered 行应置 delivered, got %s", gotDeliv.Status)
	}
	gotRead, _ := repo.GetByID(ctx, read.ID)
	if !gotRead.IsRead || gotRead.ReadAt == nil {
		t.Fatalf("read 行应置 IsRead+ReadAt, got %+v", gotRead)
	}
	gotFail, _ := repo.GetByID(ctx, failed.ID)
	if gotFail.Status != "send_failed" {
		t.Fatalf("failed 行应置 send_failed, got %s", gotFail.Status)
	}
	if v, ok := gotFail.Extra["delivery_error"].(string); !ok || v == "" {
		t.Fatalf("failed 行应记录 delivery_error, extra=%v", gotFail.Extra)
	}
}

// T3 验收②：wamid 未命中（未回写/旧占位）静默忽略；普通消息推送不受影响。
func TestDispatchWhatsAppStatuses_MissAndPassthrough(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := &WebhookService{db: db}
	ctx := context.Background()

	// 未命中：无报错、无副作用
	handled, err := svc.dispatchWhatsAppStatuses(ctx, "wa-acc-t3b", []byte(`{"entry":[{"changes":[{"value":{"statuses":[{"id":"wamid.UNKNOWN9","status":"delivered"}]}}]}]}`))
	if err != nil || !handled {
		t.Fatalf("unknown wamid 应静默处理 handled=%v err=%v", handled, err)
	}
	// 普通消息推送：无 statuses → handled=false 落回主链路
	handled, err = svc.dispatchWhatsAppStatuses(ctx, "wa-acc-t3b", []byte(`{"entry":[{"changes":[{"value":{"messages":[{"from":"86138","id":"wamid.INMSG1","timestamp":"1","type":"text","text":{"body":"hi"}}]}}]}]}`))
	if err != nil {
		t.Fatalf("msg payload err: %v", err)
	}
	if handled {
		t.Fatal("messages 推送不应被 statuses 分支吞掉")
	}
	// 非法 payload → handled=false
	handled, _ = svc.dispatchWhatsAppStatuses(ctx, "wa-acc-t3b", []byte(`not-json`))
	if handled {
		t.Fatal("invalid payload should passthrough")
	}
}

// T3 验收③：未知 status 不写入。
func TestUpdateDeliveryStatus_UnknownIgnored(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	repo := repository.NewMessageHubRepositoryWithDB(db)
	ctx := context.Background()
	row := &model.MessageHub{MsgID: "wamid.ST9", Platform: "whatsapp", AccountID: "a", Direction: "outbound", MsgType: "text", ConversationID: "c", Content: "x", SentAt: time.Now()}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.UpdateDeliveryStatus(ctx, "whatsapp", "a", "wamid.ST9", "weird_status", ""); err != nil {
		t.Fatalf("unknown status should be no-op: %v", err)
	}
	got, _ := repo.GetByID(ctx, row.ID)
	if got.Status == "weird_status" {
		t.Fatalf("unknown status must not be written")
	}
}
