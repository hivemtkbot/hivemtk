package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
)

// R54: message_inbound 规则全链验证（同步 Dispatch，修正前置污染）
func TestR54RuleInboundChain(t *testing.T) {
	testDB := testutil.NewTestDB(t,
		&model.AutomationRule{},
		&model.RulePendingExecution{},
		&model.MessageHub{},
		&model.CustomerSession{},
	)
	db.SetTestDB(testDB)
	rule := &model.AutomationRule{
		Name: "R54链", Event: RuleEventMessageInbound,
		Conditions: `[{"field":"content","op":"contains","value":"R54关键词"}]`,
		Actions:    `[{"type":"send_message","value":"R54自动回复内容"}]`,
		Enabled:    true,
	}
	if err := db.GetDB().Create(rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}
	sess := &model.CustomerSession{SessionID: "r54sess", Platform: "douyin", AccountID: "r54acc", Status: model.SessionStatusWaiting}
	if err := db.GetDB().Create(sess).Error; err != nil {
		t.Fatalf("create sess: %v", err)
	}
	svc := NewRuleEngineService()
	// R54: 同步直接调 Dispatch 路径内部逻辑定位
	var rules []*model.AutomationRule
	db.GetDB().Where("event = ? AND enabled = ?", RuleEventMessageInbound, true).Find(&rules)
	t.Logf("step1 rules=%d", len(rules))
	matched := svc.matchConditions(rules[0], sess, "")
	t.Logf("step2 match=%v", matched)
	if matched {
		svc.executeRule(context.Background(), rules[0], "r54sess", sess)
		t.Logf("step3 executeRule done")
	}
	var rules2 []*model.AutomationRule
	db.GetDB().Where("event = ?", RuleEventMessageInbound).Find(&rules2)
	t.Logf("rules found in Dispatch-path: %d", len(rules2))
	var sessChk model.CustomerSession
	if err := db.GetDB().Where("session_id = ?", "r54sess").First(&sessChk).Error; err != nil {
		t.Logf("session lookup error: %v", err)
	} else {
		t.Logf("sess.platform=%s status=%s", sessChk.Platform, sessChk.Status)
	}
	time.Sleep(2 * time.Second)
	var after model.AutomationRule
	db.GetDB().First(&after, rule.ID)
	t.Logf("run_count=%d", after.RunCount)
	var cnt int64
	db.GetDB().Model(&model.MessageHub{}).Where("content = ?", "R54自动回复内容").Count(&cnt)
	t.Logf("outbound=%d", cnt)
	if after.RunCount == 0 || cnt == 0 {
		t.Fatalf("message_inbound 规则链断裂: run_count=%d outbound=%d", after.RunCount, cnt)
	}
}
