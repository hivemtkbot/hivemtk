package service

import (
	"context"
	"testing"
	"time"
)

// ===== Customer Journey 状态机测试 =====

func TestCustomerJourney_NewCustomerIsStranger(t *testing.T) {
	s := NewCustomerJourneyService()
	state := s.GetState("cust_001")
	if state.CurrentStage != StageStranger {
		t.Errorf("new customer should be stranger, got %s", state.CurrentStage)
	}
}

func TestCustomerJourney_Transition(t *testing.T) {
	s := NewCustomerJourneyService()
	event, err := s.Transition(ctx, "cust_002", StageLead, "ai_chat", "ai", "留资成功", nil)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	if event.FromStage != StageStranger || event.ToStage != StageLead {
		t.Errorf("event stages wrong: %s -> %s", event.FromStage, event.ToStage)
	}
	state := s.GetState("cust_002")
	if state.CurrentStage != StageLead {
		t.Errorf("state not updated")
	}
}

func TestCustomerJourney_InvalidStage(t *testing.T) {
	s := NewCustomerJourneyService()
	_, err := s.Transition(context.Background(), "cust_003", JourneyStage("invalid"), "", "", "", nil)
	if err == nil {
		t.Error("should reject invalid stage")
	}
}

func TestCustomerJourney_AutoTagOnTransition(t *testing.T) {
	s := NewCustomerJourneyService()
	_, _ = s.Transition(ctx, "cust_004", StageQuoted, "ai_chat", "ai", "已报价", nil)
	state := s.GetState("cust_004")
	hasTag := false
	for _, tag := range state.AutoTags {
		if tag == "stage:quoted" {
			hasTag = true
			break
		}
	}
	if !hasTag {
		t.Errorf("expected stage:quoted tag, got %v", state.AutoTags)
	}
}

func TestCustomerJourney_Touch(t *testing.T) {
	s := NewCustomerJourneyService()
	s.Touch("cust_005", "ai_chat")
	s.Touch("cust_005", "ai_chat")
	s.Touch("cust_005", "ai_chat")
	state := s.GetState("cust_005")
	if state.TotalTouches != 3 {
		t.Errorf("expected 3 touches, got %d", state.TotalTouches)
	}
}

func TestCustomerJourney_ListByStage(t *testing.T) {
	s := NewCustomerJourneyService()
	_, _ = s.Transition(ctx, "a", StageLead, "ai", "ai", "", nil)
	_, _ = s.Transition(ctx, "b", StageLead, "ai", "ai", "", nil)
	_, _ = s.Transition(ctx, "c", StageInterested, "ai", "ai", "", nil)
	leads := s.ListByStage(StageLead)
	if len(leads) != 2 {
		t.Errorf("expected 2 leads, got %d", len(leads))
	}
}

func TestCustomerJourney_AutoDetectSleeping(t *testing.T) {
	s := NewCustomerJourneyService()
	_, _ = s.Transition(ctx, "old_cust", StageWon, "ai", "ai", "", nil)
	// 手动修改时间（用 reflection 或直接操作）
	s.mu.Lock()
	state := s.states["old_cust"]
	state.LastTouchAt = time.Now().Add(-200 * 24 * time.Hour) // 200 天前
	s.mu.Unlock()
	wokeUp := s.AutoDetectSleeping()
	found := false
	for _, cid := range wokeUp {
		if cid == "old_cust" {
			found = true
		}
	}
	if !found {
		t.Error("old_cust should be detected as sleeping")
	}
}

// ===== FollowUp 测试 =====

func TestFollowUp_Schedule(t *testing.T) {
	journey := NewCustomerJourneyService()
	svc := NewFollowUpService(journey)
	r, err := svc.Schedule(context.Background(), "cust_001", "sales_001", ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title: "首次跟进",
	})
	if err != nil {
		t.Fatalf("schedule failed: %v", err)
	}
	if r.Status != "pending" {
		t.Errorf("new reminder should be pending")
	}
}

func TestFollowUp_Complete(t *testing.T) {
	journey := NewCustomerJourneyService()
	svc := NewFollowUpService(journey)
	r, _ := svc.Schedule(context.Background(), "cust_001", "sales_001", ReminderFirstContact, 1*time.Hour, nil)
	err := svc.Complete(r.ID)
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	pending := svc.ListPending("sales_001", 0)
	for _, p := range pending {
		if p.ID == r.ID {
			t.Error("completed reminder should not be in pending")
		}
	}
}

func TestFollowUp_ListPendingByOwner(t *testing.T) {
	journey := NewCustomerJourneyService()
	svc := NewFollowUpService(journey)
	_, _ = svc.Schedule(context.Background(), "a", "sales_001", ReminderFirstContact, 1*time.Hour, nil)
	_, _ = svc.Schedule(context.Background(), "b", "sales_002", ReminderFirstContact, 1*time.Hour, nil)
	pending1 := svc.ListPending("sales_001", 0)
	if len(pending1) != 1 {
		t.Errorf("expected 1 for sales_001, got %d", len(pending1))
	}
}

func TestFollowUp_Overdue(t *testing.T) {
	journey := NewCustomerJourneyService()
	svc := NewFollowUpService(journey)
	r, _ := svc.Schedule(context.Background(), "a", "sales_001", ReminderFirstContact, -1*time.Hour, nil)
	_ = r
	overdue := svc.ListOverdue("sales_001")
	if len(overdue) == 0 {
		t.Error("expected at least 1 overdue")
	}
}

func TestFollowUp_DailyCalendar(t *testing.T) {
	journey := NewCustomerJourneyService()
	svc := NewFollowUpService(journey)
	// 使用绝对时间戳避免跨日边界问题（3 个提醒都在今天 00:00-24:00 内）
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	due1 := dayStart.Add(9 * time.Hour)  // 今天 09:00
	due2 := dayStart.Add(12 * time.Hour) // 今天 12:00
	due3 := dayStart.Add(15 * time.Hour) // 今天 15:00
	_, _ = svc.Schedule(context.Background(), "a", "sales_001", ReminderFirstContact, due1.Sub(now), nil)
	_, _ = svc.Schedule(context.Background(), "b", "sales_001", ReminderFirstContact, due2.Sub(now), nil)
	_, _ = svc.Schedule(context.Background(), "c", "sales_001", ReminderFirstContact, due3.Sub(now), nil)
	cal := svc.GetDailyCalendar("sales_001", now)
	// 3 个都在今天范围内
	if len(cal) < 3 {
		t.Errorf("expected 3 in calendar, got %d", len(cal))
	}
}

// TestFollowUp_WeeklyCalendar 验证 7 天日历分组
func TestFollowUp_WeeklyCalendar(t *testing.T) {
	journey := NewCustomerJourneyService()
	svc := NewFollowUpService(journey)
	// 1 分钟后（今天）+ 25h 后（明天）+ 73h 后（第 4 天）
	_, _ = svc.Schedule(context.Background(), "a", "sales_001", ReminderFirstContact, 1*time.Minute, nil)
	_, _ = svc.Schedule(context.Background(), "b", "sales_001", ReminderFirstContact, 25*time.Hour, nil)
	_, _ = svc.Schedule(context.Background(), "c", "sales_001", ReminderFirstContact, 73*time.Hour, nil)
	week := svc.GetWeeklyCalendar("sales_001", time.Now())
	// 至少 7 天（即便 0 个）
	if len(week) != 7 {
		t.Errorf("weekly calendar should have 7 days, got %d", len(week))
	}
	// 累计应 >= 3
	total := 0
	for _, day := range week {
		total += len(day)
	}
	if total < 3 {
		t.Errorf("expected total >= 3 reminders in week, got %d", total)
	}
}

// ===== Repurchase Engine 测试 =====

func TestRepurchase_Champion(t *testing.T) {
	e := NewRepurchaseEngine()
	// 模拟冠军客户
	now := time.Now()
	for i := 0; i < 12; i++ {
		e.RecordPurchase(PurchaseEvent{
			OrderID: "o1", CustomerID: "vip", Amount: 1500, ProductName: "光子嫩肤",
			OrderedAt: now.AddDate(0, 0, -i*5),
		})
	}
	rfm := e.ComputeRFM("vip")
	if rfm.Segment != RFMTYPEChampion {
		t.Errorf("expected Champion, got %s (R=%d F=%d M=%d)", rfm.Segment, rfm.R, rfm.F, rfm.M)
	}
	pred := e.Predict("vip")
	if pred.Probability < 0.7 {
		t.Errorf("champion should have high probability, got %f", pred.Probability)
	}
}

func TestRepurchase_Hibernating(t *testing.T) {
	e := NewRepurchaseEngine()
	now := time.Now()
	// 一年前只买过 1 次
	e.RecordPurchase(PurchaseEvent{
		OrderID: "o1", CustomerID: "old", Amount: 500, ProductName: "瑜伽课",
		OrderedAt: now.AddDate(-1, 0, 0),
	})
	rfm := e.ComputeRFM("old")
	if rfm.Segment != RFMTYPEHibernating && rfm.Segment != RFMTYPELost {
		t.Errorf("expected Hibernating or Lost, got %s", rfm.Segment)
	}
}

func TestRepurchase_Newbie(t *testing.T) {
	e := NewRepurchaseEngine()
	now := time.Now()
	e.RecordPurchase(PurchaseEvent{
		OrderID: "o1", CustomerID: "new", Amount: 200, ProductName: "试听课",
		OrderedAt: now.AddDate(0, 0, -2),
	})
	rfm := e.ComputeRFM("new")
	if rfm.Segment != RFMTYPENewbie && rfm.Segment != RFMTYPEPotential {
		t.Errorf("expected Newbie/Potential, got %s", rfm.Segment)
	}
}

func TestRepurchase_Plan(t *testing.T) {
	e := NewRepurchaseEngine()
	now := time.Now()
	e.RecordPurchase(PurchaseEvent{
		OrderID: "o1", CustomerID: "old", Amount: 200, ProductName: "x",
		OrderedAt: now.AddDate(0, 0, -120),
	})
	plan := e.GenerateReactivationPlan("old")
	if len(plan) == 0 {
		t.Error("hibernating should have reactivation plan")
	}
	// 第 1 波应该在 3 天后
	if plan[0].WaitDays != 3 {
		t.Errorf("first wave should be 3 days, got %d", plan[0].WaitDays)
	}
}

func TestRepurchase_ReactivationCandidates(t *testing.T) {
	e := NewRepurchaseEngine()
	now := time.Now()
	for i := 0; i < 3; i++ {
		cid := "old_" + string(rune('a'+i))
		e.RecordPurchase(PurchaseEvent{
			OrderID: "o1", CustomerID: cid, Amount: 100, ProductName: "x",
			OrderedAt: now.AddDate(0, -3, 0),
		})
	}
	cands := e.ListReactivationCandidates(0)
	if len(cands) == 0 {
		t.Error("expected reactivation candidates")
	}
}

// ===== 业务组件回归测试 =====
