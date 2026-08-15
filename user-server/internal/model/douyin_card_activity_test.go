package model

import (
	"testing"
)

func TestDouyinCardActivity_TableName(t *testing.T) {
	activity := &DouyinCardActivity{}
	tableName := activity.TableName()
	if tableName != "douyin_card_activities" {
		t.Errorf("Expected table name 'douyin_card_activities', got %s", tableName)
	}
}

func TestDouyinCardActivity_BasicFields(t *testing.T) {
	activity := &DouyinCardActivity{
		ID:        1,
		CardID:    100,
		UserID:    50,
		Action:    "view",
		Username:  "testuser",
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0",
	}

	if activity.ID != 1 {
		t.Errorf("Expected ID 1, got %d", activity.ID)
	}
	if activity.CardID != 100 {
		t.Errorf("Expected CardID 100, got %d", activity.CardID)
	}
	if activity.UserID != 50 {
		t.Errorf("Expected UserID 50, got %d", activity.UserID)
	}
	if activity.Action != "view" {
		t.Errorf("Expected Action 'view', got %s", activity.Action)
	}
}

func TestDouyinCardActivity_ActionValues(t *testing.T) {
	actions := []string{"view", "like", "share", "collect", "comment"}

	for _, action := range actions {
		activity := &DouyinCardActivity{
			Action: action,
		}
		if activity.Action != action {
			t.Errorf("Expected Action %s, got %s", action, activity.Action)
		}
	}
}

func TestDouyinCardActivity_WithEmptyFields(t *testing.T) {
	activity := &DouyinCardActivity{
		CardID: 100,
		UserID: 50,
		Action: "view",
	}

	if activity.Username != "" {
		t.Errorf("Expected empty Username, got %s", activity.Username)
	}
	if activity.IPAddress != "" {
		t.Errorf("Expected empty IPAddress, got %s", activity.IPAddress)
	}
	if activity.UserAgent != "" {
		t.Errorf("Expected empty UserAgent, got %s", activity.UserAgent)
	}
}

func TestKuaishouCardActivity_TableName(t *testing.T) {
	activity := &KuaishouCardActivity{}
	tableName := activity.TableName()
	if tableName != "kuaishou_card_activities" {
		t.Errorf("Expected table name 'kuaishou_card_activities', got %s", tableName)
	}
}

func TestKuaishouCardActivity_BasicFields(t *testing.T) {
	activity := &KuaishouCardActivity{
		ID:           1,
		CardID:       100,
		UserID:       50,
		Username:     "testuser",
		ActivityType: "view",
		IPAddress:    "192.168.1.1",
		UserAgent:    "Mozilla/5.0",
		ExtraData:    `{"key": "value"}`,
	}

	if activity.ID != 1 {
		t.Errorf("Expected ID 1, got %d", activity.ID)
	}
	if activity.CardID != 100 {
		t.Errorf("Expected CardID 100, got %d", activity.CardID)
	}
	if activity.ActivityType != "view" {
		t.Errorf("Expected ActivityType 'view', got %s", activity.ActivityType)
	}
}

func TestKuaishouCardActivity_ActivityTypeValues(t *testing.T) {
	types := []string{"view", "like", "share", "collect", "comment"}

	for _, activityType := range types {
		activity := &KuaishouCardActivity{
			ActivityType: activityType,
		}
		if activity.ActivityType != activityType {
			t.Errorf("Expected ActivityType %s, got %s", activityType, activity.ActivityType)
		}
	}
}

func TestXianyuCardActivity_TableName(t *testing.T) {
	activity := &XianyuCardActivity{}
	tableName := activity.TableName()
	if tableName != "xianyu_card_activities" {
		t.Errorf("Expected table name 'xianyu_card_activities', got %s", tableName)
	}
}

func TestXianyuCardActivity_BasicFields(t *testing.T) {
	activity := &XianyuCardActivity{
		ID:           1,
		CardID:       100,
		ActivityType: "view",
		IP:           "192.168.1.1",
		UserAgent:    "Mozilla/5.0",
		Referer:      "https://google.com",
		Country:      "CN",
		Province:     "Beijing",
		City:         "Beijing",
		DeviceType:   "mobile",
		OS:           "iOS",
		Browser:      "Safari",
	}

	if activity.ID != 1 {
		t.Errorf("Expected ID 1, got %d", activity.ID)
	}
	if activity.CardID != 100 {
		t.Errorf("Expected CardID 100, got %d", activity.CardID)
	}
	if activity.ActivityType != "view" {
		t.Errorf("Expected ActivityType 'view', got %s", activity.ActivityType)
	}
	if activity.IP != "192.168.1.1" {
		t.Errorf("Expected IP '192.168.1.1', got %s", activity.IP)
	}
}

func TestXianyuCardActivity_ActivityTypeValues(t *testing.T) {
	types := []string{"view", "click", "share"}

	for _, activityType := range types {
		activity := &XianyuCardActivity{
			ActivityType: activityType,
		}
		if activity.ActivityType != activityType {
			t.Errorf("Expected ActivityType %s, got %s", activityType, activity.ActivityType)
		}
	}
}

func TestXianyuCardActivity_DeviceTypeValues(t *testing.T) {
	types := []string{"mobile", "pc", "tablet"}

	for _, deviceType := range types {
		activity := &XianyuCardActivity{
			DeviceType: deviceType,
		}
		if activity.DeviceType != deviceType {
			t.Errorf("Expected DeviceType %s, got %s", deviceType, activity.DeviceType)
		}
	}
}

func TestXiaohongshuCardActivity_TableName(t *testing.T) {
	activity := &XiaohongshuCardActivity{}
	tableName := activity.TableName()
	if tableName != "xiaohongshu_card_activities" {
		t.Errorf("Expected table name 'xiaohongshu_card_activities', got %s", tableName)
	}
}

func TestXiaohongshuCardActivity_BasicFields(t *testing.T) {
	activity := &XiaohongshuCardActivity{
		ID:           1,
		CardID:       100,
		UserID:       50,
		Username:     "testuser",
		ActivityType: "view",
		Content:      "Great product!",
		IPAddress:    "192.168.1.1",
		UserAgent:    "Mozilla/5.0",
		ExtraData:    `{"key": "value"}`,
	}

	if activity.ID != 1 {
		t.Errorf("Expected ID 1, got %d", activity.ID)
	}
	if activity.CardID != 100 {
		t.Errorf("Expected CardID 100, got %d", activity.CardID)
	}
	if activity.ActivityType != "view" {
		t.Errorf("Expected ActivityType 'view', got %s", activity.ActivityType)
	}
	if activity.Content != "Great product!" {
		t.Errorf("Expected Content 'Great product!', got %s", activity.Content)
	}
}

func TestXiaohongshuCardActivity_ActivityTypeValues(t *testing.T) {
	types := []string{"view", "like", "share", "collect", "comment"}

	for _, activityType := range types {
		activity := &XiaohongshuCardActivity{
			ActivityType: activityType,
		}
		if activity.ActivityType != activityType {
			t.Errorf("Expected ActivityType %s, got %s", activityType, activity.ActivityType)
		}
	}
}

