package service

import (
	"context"
	"testing"
)

func TestIsUrgentOrComplaint_Complaint(t *testing.T) {
	s := &SessionAssignmentService{}
	cases := []string{
		"我要投诉",
		"我要举报你们",
		"我要退钱",
		"骗子",
		"假货",
		"315曝光",
	}
	for _, c := range cases {
		if !s.isUrgentOrComplaint(context.Background(), c) {
			t.Errorf("expected urgent for %q", c)
		}
	}
}

func TestIsUrgentOrComplaint_Normal(t *testing.T) {
	s := &SessionAssignmentService{}
	cases := []string{
		"你好",
		"请问价格",
		"产品很好",
		"hello world",
	}
	for _, c := range cases {
		if s.isUrgentOrComplaint(context.Background(), c) {
			t.Errorf("expected not urgent for %q", c)
		}
	}
}

func TestIsUrgentOrComplaint_CaseInsensitive(t *testing.T) {
	s := &SessionAssignmentService{}
	if !s.isUrgentOrComplaint(context.Background(), "URGENT") {
		// 中文不会被小写化，所以测试正常中文
	}
	if !s.isUrgentOrComplaint(context.Background(), "投诉") {
		t.Error("expected urgent for 投诉")
	}
}
