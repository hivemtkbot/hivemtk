package security

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestNetworkExposureGuard_PublicIPDetected_Blocks(t *testing.T) {
	guard := &NetworkExposureGuard{
		PublicBaseURL:  "http://8.8.8.8:8080",
		RequirePrivate: true,
		DialTimeout:    100 * time.Millisecond,
		dialer:         &netDialerMock{},
	}
	err := guard.Run()
	if err == nil {
		t.Fatal("expected error for public IP, got nil")
	}
}

func TestNetworkExposureGuard_PrivateIPAllowed(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"loopback", "http://127.0.0.1:8080"},
		{"private_10", "http://10.0.0.1:8080"},
		{"private_172", "http://172.16.0.1:8080"},
		{"private_192", "http://192.168.1.1:8080"},
		{"linklocal", "http://169.254.1.1:8080"},
		{"cgnet", "http://100.64.0.1:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := &NetworkExposureGuard{
				PublicBaseURL:  tt.url,
				RequirePrivate: true,
				DialTimeout:    100 * time.Millisecond,
				dialer:         &netDialerMock{},
			}
			if err := guard.Run(); err != nil {
				t.Fatalf("expected no error for %s, got: %v", tt.url, err)
			}
		})
	}
}

func TestNetworkExposureGuard_NotRequired_AlwaysPass(t *testing.T) {
	guard := &NetworkExposureGuard{
		PublicBaseURL:  "http://8.8.8.8:8080",
		RequirePrivate: false,
		DialTimeout:    100 * time.Millisecond,
		dialer:         &netDialerMock{},
	}
	if err := guard.Run(); err != nil {
		t.Fatalf("expected no error when RequirePrivate=false, got: %v", err)
	}
}

type netDialerMock struct{}

func (m *netDialerMock) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	return nil, nil
}
