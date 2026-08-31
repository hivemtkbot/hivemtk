package service

import (
	"context"
	"time"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

func TestR54RuleEngineDispatchDebug(t *testing.T) {
	_ = db.GetDB()
	rule := &model.AutomationRule{
		Name:  "R54debug",
		Event: RuleEventMessageInbound,
		Conditions: `[{"field":"content","op":"contains","value":"R54关键词"}]`,
		Actions:    `[{"type":"send_message","value":"R54自动回复内容"}]`,
		Enabled:    true,
	}
	db.GetDB().Where("name = ?", "R54debug").Delete(&model.AutomationRule{})
	if err := db.GetDB().Create(rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}
	sess := &model.CustomerSession{SessionID: "sess_1786878058431964190_9579dcd6", Platform: "douyin", AccountID: "r54acc", Status: model.SessionStatusWaiting}
	svc := NewRuleEngineService()
	svc.Dispatch(context.Background(), RuleEventMessageInbound, sess.SessionID, sess)
	time.Sleep(3 * time.Second)
	var after model.AutomationRule
	db.GetDB().First(&after, rule.ID)
	t.Logf("run_count=%d", after.RunCount)
	var cnt int64
	db.GetDB().Model(&model.MessageHub{}).Where("content = ?", "R54自动回复内容").Count(&cnt)
	t.Logf("outbound rows=%d", cnt)
	if after.RunCount == 0 {
		t.Fatalf("规则未执行")
	}
}
