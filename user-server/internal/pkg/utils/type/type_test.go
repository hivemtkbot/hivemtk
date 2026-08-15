package _type

import (
	"testing"
)

func TestUserStatusType(t *testing.T) {
	if UserStatusValid != 1 {
		t.Errorf("Expected UserStatusValid to be 1, got %d", UserStatusValid)
	}
	if UserStatusInvalid != 0 {
		t.Errorf("Expected UserStatusInvalid to be 0, got %d", UserStatusInvalid)
	}
}

func TestOrderStatusType(t *testing.T) {
	if OrderStatusPending != 0 {
		t.Errorf("Expected OrderStatusPending to be 0, got %d", OrderStatusPending)
	}
	if OrderStatusSuccess != 1 {
		t.Errorf("Expected OrderStatusSuccess to be 1, got %d", OrderStatusSuccess)
	}
	if OrderStatusForceSuccess != 2 {
		t.Errorf("Expected OrderStatusForceSuccess to be 2, got %d", OrderStatusForceSuccess)
	}
	if OrderStatusTimeout != -1 {
		t.Errorf("Expected OrderStatusTimeout to be -1, got %d", OrderStatusTimeout)
	}
	if OrderStatusForceClose != -2 {
		t.Errorf("Expected OrderStatusForceClose to be -2, got %d", OrderStatusForceClose)
	}
}

func TestAccountStatusType(t *testing.T) {
	if AccountStatusActive != 1 {
		t.Errorf("Expected AccountStatusActive to be 1, got %d", AccountStatusActive)
	}
	if AccountStatusInactive != 0 {
		t.Errorf("Expected AccountStatusInactive to be 0, got %d", AccountStatusInactive)
	}
}

func TestProxy(t *testing.T) {
	p := Proxy{
		EnableProxy: true,
		Protocol:    "http",
		Host:        "localhost",
		Port:        8080,
	}

	if !p.EnableProxy {
		t.Error("Expected EnableProxy to be true")
	}
	if p.Protocol != "http" {
		t.Errorf("Expected Protocol to be 'http', got %s", p.Protocol)
	}
	if p.Host != "localhost" {
		t.Errorf("Expected Host to be 'localhost', got %s", p.Host)
	}
	if p.Port != 8080 {
		t.Errorf("Expected Port to be 8080, got %d", p.Port)
	}
}

func TestHeaders(t *testing.T) {
	h := make(Headers)
	h["Content-Type"] = "application/json"
	h["Authorization"] = "Bearer token"

	if h["Content-Type"] != "application/json" {
		t.Errorf("Expected Content-Type to be 'application/json', got %s", h["Content-Type"])
	}
	if h["Authorization"] != "Bearer token" {
		t.Errorf("Expected Authorization to be 'Bearer token', got %s", h["Authorization"])
	}
}

func TestSmlistType(t *testing.T) {
	if SmlistTypeGirlSlriGistType != 1 {
		t.Errorf("Expected SmlistTypeGirlSlriGistType to be 1, got %d", SmlistTypeGirlSlriGistType)
	}
	if SmlistTypeGirlMlriGype != 2 {
		t.Errorf("Expected SmlistTypeGirlMlriGype to be 2, got %d", SmlistTypeGirlMlriGype)
	}
	if SmlistTypeManSnaMistType != 3 {
		t.Errorf("Expected SmlistTypeManSnaMistType to be 3, got %d", SmlistTypeManSnaMistType)
	}
	if SmlistTypeManMnaM != 4 {
		t.Errorf("Expected SmlistTypeManMnaM to be 4, got %d", SmlistTypeManMnaM)
	}
}

