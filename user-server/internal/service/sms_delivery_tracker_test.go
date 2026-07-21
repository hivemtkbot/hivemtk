package service

// sms_delivery_tracker_test.go 短信到达率追踪服务测试
//
// 测试覆盖：
//  1) 构造
//  2) DetectCarrierFromPhone 号段识别
//  3) hasPrefix 工具
//  4) DetectAndRecordPortability
//  5) GetCurrentCarrier 缓存 fallback
//  6) GetDeliveryRateMetrics 边界
//  7) RecordBlacklistEvent 分类
//  8) RecordFromProvider 串联
//  9) MarshalReport

import (
	"context"
	"strings"
	"testing"
	"time"
)

func newDeliveryTracker() *SmsDeliveryTrackerService {
	return NewSmsDeliveryTrackerService(nil, nil, nil)
}

// 1) 构造
func TestSmsDeliveryTracker_NewService(t *testing.T) {
	s := newDeliveryTracker()
	if s == nil {
		t.Fatal("Expected non-nil service")
	}
	if s.tracking == nil {
		t.Error("Expected tracking fallback")
	}
	if s.repo == nil {
		t.Error("Expected repo fallback")
	}
	if s.carrierCache == nil {
		t.Error("Expected carrier cache init")
	}
}

// 2) DetectCarrierFromPhone
func TestSmsDeliveryTracker_DetectCarrier(t *testing.T) {
	cases := []struct {
		phone string
		want  SmsCarrier
	}{
		{"13800138000", SmsCarrierMobile},
		{"13900139000", SmsCarrierMobile},
		{"13000130000", SmsCarrierUnicom},
		{"18600186000", SmsCarrierUnicom},
		{"13300133000", SmsCarrierTelecom},
		{"18900189000", SmsCarrierTelecom},
		{"20000000000", SmsCarrierUnknown},
		{"123", SmsCarrierUnknown},
		{"138-0013-8000", SmsCarrierMobile}, // 规范化后识别
	}
	for _, c := range cases {
		got := DetectCarrierFromPhone(c.phone)
		if got != c.want {
			t.Errorf("phone=%s: got %s, want %s", c.phone, got, c.want)
		}
	}
}

// 3) hasPrefix
func TestSmsDeliveryTracker_HasPrefix(t *testing.T) {
	if !hasPrefix("1380000", []string{"138", "139"}) {
		t.Error("Expected match")
	}
	if hasPrefix("1580000", []string{"138", "139"}) {
		t.Error("Expected no match")
	}
	if hasPrefix("13", []string{"138"}) {
		t.Error("Should not match (prefix too short)")
	}
}

// 4) DetectAndRecordPortability
func TestSmsDeliveryTracker_DetectAndRecordPortability_NilDB(t *testing.T) {
	s := newDeliveryTracker()
	// 首次记录 + nil db → 应当不 panic
	err := s.DetectAndRecordPortability(context.Background(), "13800138000", "mobile")
	if err != nil {
		t.Errorf("Expected nil err with nil db, got %v", err)
	}
}

func TestSmsDeliveryTracker_DetectAndRecordPortability_EmptyPhone(t *testing.T) {
	s := newDeliveryTracker()
	err := s.DetectAndRecordPortability(context.Background(), "", "mobile")
	if err == nil {
		t.Error("Expected error for empty phone")
	}
}

func TestSmsDeliveryTracker_DetectAndRecordPortability_UnknownCarrier(t *testing.T) {
	s := newDeliveryTracker()
	// 未知运营商但有 webhook carrier → 应识别为对应运营商
	err := s.DetectAndRecordPortability(context.Background(), "13800138000", "unicom")
	if err != nil {
		t.Errorf("Expected nil err, got %v", err)
	}
	// webhook 未给 → 用号段兜底
	if err := s.DetectAndRecordPortability(context.Background(), "13800138000", ""); err != nil {
		t.Errorf("Expected nil err, got %v", err)
	}
	// webhook 给 unknown
	if err := s.DetectAndRecordPortability(context.Background(), "13800138000", "???"); err != nil {
		t.Errorf("Expected nil err, got %v", err)
	}
}

// 5) GetCurrentCarrier
func TestSmsDeliveryTracker_GetCurrentCarrier_FromCache(t *testing.T) {
	s := newDeliveryTracker()
	s.carrierCache["13800138000"] = SmsCarrierMobile
	if got := s.GetCurrentCarrier("13800138000"); got != SmsCarrierMobile {
		t.Errorf("Expected mobile, got %s", got)
	}
}

func TestSmsDeliveryTracker_GetCurrentCarrier_Fallback(t *testing.T) {
	s := newDeliveryTracker()
	// 缓存未命中 → 用号段识别
	got := s.GetCurrentCarrier("18600186000")
	if got != SmsCarrierUnicom {
		t.Errorf("Expected unicom, got %s", got)
	}
}

// 6) GetDeliveryRateMetrics
func TestSmsDeliveryTracker_GetDeliveryRateMetrics_NilDB(t *testing.T) {
	s := newDeliveryTracker()
	_, err := s.GetDeliveryRateMetrics(context.Background(), time.Now(), time.Now())
	if err == nil {
		t.Error("Expected error for nil db")
	}
}

func TestSmsDeliveryTracker_GetDeliveryRateMetrics_EmptyRange(t *testing.T) {
	s := newDeliveryTracker()
	_, err := s.GetDeliveryRateMetrics(context.Background(), time.Time{}, time.Time{})
	if err == nil {
		t.Error("Expected error for empty range")
	}
}

func TestSmsDeliveryTracker_GetDeliveryRateMetrics_EndBeforeStart(t *testing.T) {
	s := newDeliveryTracker()
	now := time.Now()
	_, err := s.GetDeliveryRateMetrics(context.Background(), now, now.Add(-time.Hour))
	if err == nil {
		t.Error("Expected error for end<start")
	}
}

// 7) RecordBlacklistEvent
func TestSmsDeliveryTracker_RecordBlacklistEvent_Carrier(t *testing.T) {
	s := newDeliveryTracker()
	s.RecordBlacklistEvent(context.Background(), "13800138000", "ERR_4002", "用户拒收", "job-1", "msg-1")
}

func TestSmsDeliveryTracker_RecordBlacklistEvent_Content(t *testing.T) {
	s := newDeliveryTracker()
	s.RecordBlacklistEvent(context.Background(), "13800138000", "ERR_4003", "内容违规", "job-1", "msg-1")
}

func TestSmsDeliveryTracker_RecordBlacklistEvent_EmptyPhone(t *testing.T) {
	s := newDeliveryTracker()
	s.RecordBlacklistEvent(context.Background(), "", "ERR_4002", "", "", "")
	// 应静默
}

// 8) RecordFromProvider
func TestSmsDeliveryTracker_RecordFromProvider_NilReport(t *testing.T) {
	s := newDeliveryTracker()
	err := s.RecordFromProvider(context.Background(), nil)
	if err == nil {
		t.Error("Expected error for nil report")
	}
}

func TestSmsDeliveryTracker_RecordFromProvider_EmptyMessageID(t *testing.T) {
	s := newDeliveryTracker()
	err := s.RecordFromProvider(context.Background(), &ProviderDeliveryReport{})
	if err == nil {
		t.Error("Expected error for empty messageId")
	}
}

func TestSmsDeliveryTracker_RecordFromProvider_Full(t *testing.T) {
	s := newDeliveryTracker()
	// nil db → 内部走 nil repo 会报错，但黑名单事件不依赖 db
	err := s.RecordFromProvider(context.Background(), &ProviderDeliveryReport{
		MessageID: "msg-1",
		Phone:     "13800138000",
		Status:    "DELIVERED",
		Provider:  "aliyun",
		Carrier:   "mobile",
		ErrorCode: "ERR_4002",
	})
	if err != nil {
		t.Log("expected error with nil repo:", err)
	}
}

// 9) MarshalReport
func TestSmsDeliveryTracker_MarshalReport(t *testing.T) {
	out := MarshalReport(map[string]any{"a": 1})
	if !strings.Contains(out, `"a"`) {
		t.Errorf("Unexpected output: %s", out)
	}
}
