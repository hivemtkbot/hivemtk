package cron

import (
	"testing"
)

func TestNewLiveCodeRotator(t *testing.T) {
	rotator := NewLiveCodeRotator(nil)
	if rotator == nil {
		t.Error("Expected non-nil LiveCodeRotator")
	}
	if rotator.liveCodeService != nil {
		t.Error("Expected nil liveCodeService")
	}
}

