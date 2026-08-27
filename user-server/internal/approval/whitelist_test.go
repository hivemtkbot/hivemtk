package approval

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/featureflag"
)

func setFlag(t *testing.T, name string, on bool) {
	t.Helper()
	featureflag.Get(name)
	os.Setenv("FF_"+strings.ToUpper(name), boolToEnv(on))
	featureflag.DefaultManager().ReloadAll()
}

func boolToEnv(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func TestBootstrapFlag(t *testing.T) {
	setFlag(t, FlagKey, false)
	if featureflag.Get(FlagKey).Bool() {
		t.Fatalf("expected default off for tool_approval_gate")
	}
}

func TestDefaultDeny(t *testing.T) {
	setFlag(t, FlagKey, true)
	defer setFlag(t, FlagKey, false)
	w := NewWhiteList(nil, time.Now)
	decisions := 0
	w.callback = func(ctx context.Context, tn, acc string, d Decision) { decisions++ }
	if w.IsApproved(context.Background(), "reach.proactive.whatsapp", "acct1") {
		t.Fatalf("expected deny by default")
	}
	if decisions != 1 {
		t.Fatalf("expected 1 callback")
	}
}

func TestWhitelistAllows(t *testing.T) {
	setFlag(t, FlagKey, true)
	defer setFlag(t, FlagKey, false)
	w := NewWhiteList(nil, time.Now)
	w.Whitelist("reach.proactive.whatsapp", "acct1", time.Time{})
	if !w.IsApproved(context.Background(), "reach.proactive.whatsapp", "acct1") {
		t.Fatalf("expected allow after whitelist")
	}
	if w.IsApproved(context.Background(), "reach.proactive.whatsapp", "acct2") {
		t.Fatalf("expected deny for other account")
	}
}

func TestWhitelistExpiry(t *testing.T) {
	setFlag(t, FlagKey, true)
	defer setFlag(t, FlagKey, false)
	now := time.Now()
	w := NewWhiteList(nil, func() time.Time { return now })
	w.Whitelist("reach.batch_send.email", "acct1", now.Add(-time.Minute))
	if w.IsApproved(context.Background(), "reach.batch_send.email", "acct1") {
		t.Fatalf("expected deny after expiry")
	}
}

func TestFlagOffAllDenied(t *testing.T) {
	setFlag(t, FlagKey, false)
	w := NewWhiteList(nil, time.Now)
	w.Whitelist("reach.dm.telegram", "acct1", time.Time{})
	if w.IsApproved(context.Background(), "reach.dm.telegram", "acct1") {
		t.Fatalf("expected deny when flag off")
	}
}

func TestDecisionCallbackThreadSafety(t *testing.T) {
	setFlag(t, FlagKey, true)
	defer setFlag(t, FlagKey, false)
	var (
		mu  sync.Mutex
		cnt int
	)
	w := NewWhiteList(func(ctx context.Context, tn, acc string, d Decision) {
		mu.Lock()
		cnt++
		mu.Unlock()
	}, time.Now)
	w.Whitelist("reach.proactive.whatsapp", "acct1", time.Time{})
	for i := 0; i < 50; i++ {
		w.IsApproved(context.Background(), "reach.proactive.whatsapp", "acct1")
	}
	if cnt != 50 {
		t.Fatalf("expected 50 callbacks, got %d", cnt)
	}
}
