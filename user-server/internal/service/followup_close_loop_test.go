package service

import (
	"context"
	"strings"
	"testing"
	"time"

)


// TestFollowUp_CompleteWithResult_StageAdvance 跟进结果推进客户旅程
func TestFollowUp_CompleteWithResult_StageAdvance(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	stats := setupE2EStats(t)
	followup.SetStats(context.Background(), stats)

	custID := "cust_close_001"
	ownerID := "sales_001"

	state0 := journey.GetState(context.Background(), custID)
	if state0.CurrentStage != StageStranger {
		t.Fatalf("初始阶段应为陌生，实际: %s", state0.CurrentStage)
	}

	r, err := followup.Schedule(context.Background(), custID, ownerID, ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title:    "首次跟进",
		Priority: PriorityHigh,
	})
	if err != nil {
		t.Fatalf("安排跟进失败: %v", err)
	}

	err = followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultInterested, "客户咨询了价格")
	if err != nil {
		t.Fatalf("完成跟进失败: %v", err)
	}

	state1 := journey.GetState(context.Background(), custID)
	if state1.CurrentStage != StageInterested {
		t.Errorf("客户旅程应推进到 %s，实际: %s", StageInterested, state1.CurrentStage)
	}
	if state1.TotalTouches < 1 {
		t.Error("互动次数应增加")
	}
	t.Logf("✅ 跟进结果%q → 客户旅程推进到: %s", FollowUpResultInterested, state1.CurrentStage)
}

// TestFollowUp_CompleteWithResult_Quoted 报价后推进到报价阶段
func TestFollowUp_CompleteWithResult_Quoted(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)

	custID := "cust_close_002"
	ownerID := "sales_002"
	_, _ = journey.Transition(context.Background(), custID, StageLead, "test", ownerID, "测试", nil)

	r, _ := followup.Schedule(context.Background(), custID, ownerID, ReminderQuoteFollowup, 1*time.Hour, &ScheduleOptions{
		Title: "报价", Priority: PriorityHigh,
	})
	if err := followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultQuoted, "已发送报价单"); err != nil {
		t.Fatalf("完成跟进失败: %v", err)
	}
	state := journey.GetState(context.Background(), custID)
	if state.CurrentStage != StageQuoted {
		t.Errorf("应推进到 %s，实际: %s", StageQuoted, state.CurrentStage)
	}
	t.Logf("✅ 报价跟进 → 推进到: %s", state.CurrentStage)
}

// TestFollowUp_CompleteWithResult_Converted 成交后推进到成交阶段
func TestFollowUp_CompleteWithResult_Converted(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)

	custID := "cust_close_003"
	ownerID := "sales_003"
	_, _ = journey.Transition(context.Background(), custID, StageQuoted, "test", ownerID, "已报价", nil)

	r, _ := followup.Schedule(context.Background(), custID, ownerID, ReminderQuoteFollowup, 1*time.Hour, &ScheduleOptions{
		Title: "催单", Priority: PriorityUrgent,
	})
	if err := followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultConverted, "客户付款了 ¥9800"); err != nil {
		t.Fatalf("完成跟进失败: %v", err)
	}
	state := journey.GetState(context.Background(), custID)
	if state.CurrentStage != StageWon {
		t.Errorf("应推进到 %s，实际: %s", StageWon, state.CurrentStage)
	}
	t.Logf("✅ 成交跟进 → 推进到: %s", state.CurrentStage)
}

// TestFollowUp_CompleteWithResult_Lost 拒绝后进入流失
func TestFollowUp_CompleteWithResult_Lost(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)

	custID := "cust_close_004"
	ownerID := "sales_004"
	_, _ = journey.Transition(context.Background(), custID, StageInterested, "test", ownerID, "客户咨询", nil)

	r, _ := followup.Schedule(context.Background(), custID, ownerID, ReminderQuoteFollowup, 1*time.Hour, &ScheduleOptions{
		Title: "催单", Priority: PriorityNormal,
	})
	if err := followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultLost, "客户选择竞品"); err != nil {
		t.Fatalf("完成跟进失败: %v", err)
	}
	state := journey.GetState(context.Background(), custID)
	if state.CurrentStage != StageLost {
		t.Errorf("应推进到 %s，实际: %s", StageLost, state.CurrentStage)
	}
	t.Logf("✅ 流失跟进 → 推进到: %s", state.CurrentStage)
}

// TestFollowUp_CompleteWithResult_NoResponse 未回应进入沉睡
func TestFollowUp_CompleteWithResult_NoResponse(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)

	custID := "cust_close_005"
	ownerID := "sales_005"
	_, _ = journey.Transition(context.Background(), custID, StageContact, "test", ownerID, "已联系", nil)

	r, _ := followup.Schedule(context.Background(), custID, ownerID, ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title: "二次跟进", Priority: PriorityNormal,
	})
	if err := followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultNoResponse, "客户未回消息"); err != nil {
		t.Fatalf("完成跟进失败: %v", err)
	}
	state := journey.GetState(context.Background(), custID)
	if state.CurrentStage != StageSleeping {
		t.Errorf("应推进到 %s，实际: %s", StageSleeping, state.CurrentStage)
	}
	t.Logf("✅ 未回应 → 推进到: %s", state.CurrentStage)
}

// TestFollowUp_CompleteWithResult_DashboardRealtime 仪表盘实时更新
func TestFollowUp_CompleteWithResult_DashboardRealtime(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	stats := setupE2EStats(t)
	followup.SetStats(context.Background(), stats)

	ownerID := "sales_006"
	cust1 := "cust_dash_001"
	cust2 := "cust_dash_002"

	r1, _ := followup.Schedule(context.Background(), cust1, ownerID, ReminderQuoteFollowup, 1*time.Hour, &ScheduleOptions{
		Title: "催单", Priority: PriorityHigh,
	})
	if err := followup.CompleteWithResult(context.Background(), r1.ID, FollowUpResultConverted, "成交"); err != nil {
		t.Fatalf("完成跟进1失败: %v", err)
	}

	r2, _ := followup.Schedule(context.Background(), cust2, ownerID, ReminderQuoteFollowup, 1*time.Hour, &ScheduleOptions{
		Title: "催单", Priority: PriorityHigh,
	})
	if err := followup.CompleteWithResult(context.Background(), r2.ID, FollowUpResultQuoted, "报价"); err != nil {
		t.Fatalf("完成跟进2失败: %v", err)
	}

	perf := stats.GetSalesPerformance(context.Background(), ownerID, time.Time{})
	if perf == nil {
		t.Fatal("应能查询到该销售的业绩")
	}
	if perf.TotalFollowUps < 2 {
		t.Errorf("跟进总数应≥2，实际: %d", perf.TotalFollowUps)
	}
	t.Logf("✅ 仪表盘实时：销售 %s 跟进数=%d", ownerID, perf.TotalFollowUps)
}

// TestFollowUp_CompleteWithResult_DashboardNil 仪表盘 nil 时安全
func TestFollowUp_CompleteWithResult_DashboardNil(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)

	custID := "cust_safe_001"
	ownerID := "sales_safe"

	r, _ := followup.Schedule(context.Background(), custID, ownerID, ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title: "测试",
	})
	if err := followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultInterested, "test"); err != nil {
		t.Errorf("nil stats 不应 panic: %v", err)
	}
	t.Logf("✅ nil stats 安全")
}

// TestFollowUp_CompleteWithResult_NotFound 不存在的跟进
func TestFollowUp_CompleteWithResult_NotFound(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	err := followup.CompleteWithResult(context.Background(), "not_exist_id", FollowUpResultInterested, "")
	if err == nil {
		t.Error("不存在的跟进 ID 应报错")
	}
	t.Logf("✅ 不存在 ID 报错：%v", err)
}

// TestFollowUp_CompleteWithResult_DefaultContacted 默认跟进结果
func TestFollowUp_CompleteWithResult_DefaultContacted(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)

	custID := "cust_def_001"
	ownerID := "sales_def"

	r, _ := followup.Schedule(context.Background(), custID, ownerID, ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title: "首次",
	})
	if err := followup.Complete(context.Background(), r.ID); err != nil {
		t.Fatalf("Complete 失败: %v", err)
	}
	state := journey.GetState(context.Background(), custID)
	if state.CurrentStage != StageContact {
		t.Errorf("默认跟进结果应推进到 %s，实际: %s", StageContact, state.CurrentStage)
	}
	t.Logf("✅ 默认 Contacted → 推进到: %s", state.CurrentStage)
}

// TestFollowUp_CompleteWithResult_NoteRecorded 备注被记录
func TestFollowUp_CompleteWithResult_NoteRecorded(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)

	custID := "cust_note_001"
	ownerID := "sales_note"

	r, _ := followup.Schedule(context.Background(), custID, ownerID, ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title:       "首次",
		Description: "原始描述",
	})
	if err := followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultInterested, "客户问了价格和效果"); err != nil {
		t.Fatalf("完成失败: %v", err)
	}

	followup.mu.RLock()
	got := followup.reminders[r.ID]
	followup.mu.RUnlock()

	if got == nil {
		t.Fatal("跟进记录不存在")
	}
	if !strings.Contains(got.Description, "客户问了价格和效果") {
		t.Error("跟进备注应被记录")
	}
	if !strings.Contains(got.Description, "interested") {
		t.Error("跟进结果应被记录")
	}
	t.Logf("✅ 备注已记录")
}

// TestFollowUp_CompleteWithResult_MultipleCustomers 多客户并发
func TestFollowUp_CompleteWithResult_MultipleCustomers(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	stats := setupE2EStats(t)
	followup.SetStats(context.Background(), stats)

	ownerID := "sales_multi"
	for i := 0; i < 10; i++ {
		custID := "cust_multi_" + intToStr(i)
		r, _ := followup.Schedule(context.Background(), custID, ownerID, ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
			Title: "跟进", Priority: PriorityNormal,
		})
		switch i % 3 {
		case 0:
			_ = followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultInterested, "")
		case 1:
			_ = followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultQuoted, "")
		case 2:
			_ = followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultLost, "")
		}
	}

	interestedCount, quotedCount, lostCount := 0, 0, 0
	for i := 0; i < 10; i++ {
		custID := "cust_multi_" + intToStr(i)
		state := journey.GetState(context.Background(), custID)
		switch state.CurrentStage {
		case StageInterested:
			interestedCount++
		case StageQuoted:
			quotedCount++
		case StageLost:
			lostCount++
		}
	}
	if interestedCount == 0 || quotedCount == 0 || lostCount == 0 {
		t.Errorf("三种结果都应有客户：interested=%d, quoted=%d, lost=%d", interestedCount, quotedCount, lostCount)
	}
	t.Logf("✅ 多客户并发：interested=%d, quoted=%d, lost=%d", interestedCount, quotedCount, lostCount)
}

// TestFollowUp_CompleteWithResult_FunnelTracking 漏斗统计
func TestFollowUp_CompleteWithResult_FunnelTracking(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)

	for i := 0; i < 50; i++ {
		custID := "funnel_c" + intToStr(i)
		r, _ := followup.Schedule(context.Background(), custID, "sales_funnel", ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
			Title: "首次",
		})
		_ = followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultContacted, "")
	}
	for i := 50; i < 80; i++ {
		custID := "funnel_i" + intToStr(i)
		r, _ := followup.Schedule(context.Background(), custID, "sales_funnel", ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
			Title: "首次",
		})
		_ = followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultInterested, "")
	}
	for i := 80; i < 95; i++ {
		custID := "funnel_q" + intToStr(i)
		r, _ := followup.Schedule(context.Background(), custID, "sales_funnel", ReminderQuoteFollowup, 1*time.Hour, &ScheduleOptions{
			Title: "报价",
		})
		_ = followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultQuoted, "")
	}
	for i := 95; i < 100; i++ {
		custID := "funnel_w" + intToStr(i)
		r, _ := followup.Schedule(context.Background(), custID, "sales_funnel", ReminderQuoteFollowup, 1*time.Hour, &ScheduleOptions{
			Title: "催单",
		})
		_ = followup.CompleteWithResult(context.Background(), r.ID, FollowUpResultConverted, "")
	}

	contactedCount := 0
	interestedCount := 0
	quotedCount := 0
	wonCount := 0
	for i := 0; i < 100; i++ {
		var custID string
		switch {
		case i < 50:
			custID = "funnel_c" + intToStr(i)
		case i < 80:
			custID = "funnel_i" + intToStr(i)
		case i < 95:
			custID = "funnel_q" + intToStr(i)
		default:
			custID = "funnel_w" + intToStr(i)
		}
		state := journey.GetState(context.Background(), custID)
		switch state.CurrentStage {
		case StageContact:
			contactedCount++
		case StageInterested:
			interestedCount++
		case StageQuoted:
			quotedCount++
		case StageWon:
			wonCount++
		}
	}
	if contactedCount != 50 || interestedCount != 30 || quotedCount != 15 || wonCount != 5 {
		t.Errorf("漏斗错位：c=%d, i=%d, q=%d, w=%d", contactedCount, interestedCount, quotedCount, wonCount)
	}
	t.Logf("✅ 漏斗正确：%d → %d → %d → %d (50/30/15/5)", contactedCount, interestedCount, quotedCount, wonCount)
}



func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	buf := [20]byte{}
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

