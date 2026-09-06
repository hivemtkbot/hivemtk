package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
)

func TestMemoryFacadePickLatestValid(t *testing.T) {
	asOf := time.Now()
	old := MemoryFact{Key: "budget", Value: "5000", ValidFrom: asOf.Add(-48 * time.Hour), Confidence: 0.6}
	new := MemoryFact{Key: "budget", Value: "8000", ValidFrom: asOf.Add(-1 * time.Hour), Confidence: 0.6}

	best, ok := pickLatestValid([]MemoryFact{old, new}, asOf)
	if !ok || best.Value != "8000" {
		t.Fatalf("应取 ValidFrom 最新者，got %+v ok=%v", best, ok)
	}
	if best.Confidence != 1.0 {
		t.Errorf("双来源佐证置信度求和应封顶 1.0，got %v", best.Confidence)
	}

	best, ok = pickLatestValid([]MemoryFact{old}, asOf)
	if !ok || best.Value != "5000" || best.Confidence != 0.6 {
		t.Errorf("单条应保留原置信度，got %+v ok=%v", best, ok)
	}

	stale := MemoryFact{Key: "k", Value: "v", ValidFrom: asOf.Add(-48 * time.Hour), InvalidAt: ptrTime(asOf.Add(-24 * time.Hour))}
	if _, ok := pickLatestValid([]MemoryFact{stale}, asOf); ok {
		t.Error("全部候选失效时应返回 false")
	}

	if _, ok := pickLatestValid(nil, asOf); ok {
		t.Error("空候选应返回 false")
	}
}

func TestMemoryFacadeFactKeyOfItem(t *testing.T) {
	it := model.MemoryItem{ItemType: "fact:budget", Metadata: model.JSONMap{"key": "budget"}}
	if got := factKeyOfItem(it); got != "budget" {
		t.Errorf("metadata.key 优先，got %q", got)
	}
	it.Metadata = nil
	if got := factKeyOfItem(it); got != "budget" {
		t.Errorf("无 metadata 时应剥离 fact: 前缀，got %q", got)
	}
	it.ItemType = "summary"
	if got := factKeyOfItem(it); got != "summary" {
		t.Errorf("非 fact 前缀原样返回，got %q", got)
	}
}

func TestMemoryFacadeSplitFactKV(t *testing.T) {
	k, v := splitFactKV("budget=8000元")
	if k != "budget" || v != "8000元" {
		t.Errorf("k=v 拆分错误，got %q %q", k, v)
	}
	k, v = splitFactKV("没有等号的内容")
	if k != "" || v != "没有等号的内容" {
		t.Errorf("无 = 时整体作为 value，got %q %q", k, v)
	}
	k, v = splitFactKV("=开头")
	if k != "" || v != "=开头" {
		t.Errorf("= 在首位视为无 key，got %q %q", k, v)
	}
}

func TestMemoryFacadeNilReceiver(t *testing.T) {
	var f *MemoryFacade
	if err := f.Write(context.Background(), MemoryWrite{Scope: MemoryScopeFact}); err != nil {
		t.Errorf("nil 接收者 Write 应静默，got %v", err)
	}
	facts, err := f.Read(context.Background(), MemoryQuery{CustomerID: "c-1"})
	if err != nil || facts != nil {
		t.Errorf("nil 接收者 Read 应返回 nil,nil，got %v %v", facts, err)
	}
}

func TestMemoryFacadeNilSystems(t *testing.T) {
	f := NewMemoryFacade(nil, nil, nil)
	if err := f.Write(context.Background(), MemoryWrite{Scope: MemoryScopeDialogue, SessionID: "s-1"}); err != nil {
		t.Errorf("底层缺失时 Write 应静默跳过，got %v", err)
	}
	if err := f.Write(context.Background(), MemoryWrite{Scope: MemoryScopeFact, CustomerID: "c-1", Key: "k", Value: "v"}); err != nil {
		t.Errorf("底层缺失时 Write 应静默跳过，got %v", err)
	}
	facts, err := f.Read(context.Background(), MemoryQuery{CustomerID: "c-1"})
	if err != nil || facts != nil {
		t.Errorf("ms 缺失时 Read 应返回 nil,nil，got %v %v", facts, err)
	}
}

func TestMemoryFacadeWriteUnknownScope(t *testing.T) {
	f := NewMemoryFacade(nil, nil, nil)
	if err := f.Write(context.Background(), MemoryWrite{Scope: "nope"}); err == nil {
		t.Error("未知 scope 应返回错误")
	}
}

func TestMemoryFacadeWriteFactNilRepo(t *testing.T) {

	f := NewMemoryFacade(nil, &MemorySystem{}, nil)
	if err := f.Write(context.Background(), MemoryWrite{Scope: MemoryScopeFact, CustomerID: "c-1", Key: "budget", Value: "8000"}); err != nil {
		t.Errorf("repo 为 nil 应静默，got %v", err)
	}
}

func TestValidFactsFilterTimeAxis(t *testing.T) {
	now := time.Now()
	created := now.Add(-72 * time.Hour)

	cases := []struct {
		name                 string
		validFrom, createdAt time.Time
		invalidAt            time.Time
		asOf                 time.Time
		want                 bool
	}{
		{"无效后失效", created, created, now.Add(-time.Hour), now, false},
		{"asOf 早于失效仍有效", created, created, now.Add(time.Hour), now, true},
		{"未失效长期有效", created, created, time.Time{}, now, true},
		{"validFrom 晚于 asOf", now.Add(time.Hour), created, time.Time{}, now, false},
		{"validFrom 为零兜底 createdAt", time.Time{}, created, time.Time{}, now, true},
		{"invalidAt 恰等于 asOf 视为已失效", created, created, now, now, false},
	}
	for _, c := range cases {
		if got := validAtAsOf(c.validFrom, c.createdAt, c.invalidAt, c.asOf); got != c.want {
			t.Errorf("%s: validAtAsOf=%v, want %v", c.name, got, c.want)
		}
	}

	if got := effectiveValidFrom(time.Time{}, created); !got.Equal(created) {
		t.Errorf("effectiveValidFrom 零值应兜底 createdAt，got %v", got)
	}
	if got := effectiveValidFrom(now, created); !got.Equal(now) {
		t.Errorf("effectiveValidFrom 非零应原样返回，got %v", got)
	}
}

func TestListValidFactsNilRepo(t *testing.T) {
	m := &MemorySystem{}
	facts, err := m.ListValidFacts(context.Background(), "c-1", time.Time{}, 10)
	if err != nil || facts != nil {
		t.Errorf("repo 为 nil 应返回 nil,nil，got %v %v", facts, err)
	}
}

func TestListValidFactsAsOfZeroOnNilRepo(t *testing.T) {

	m := &MemorySystem{}
	if _, err := m.L2ListFactsAsOf(context.Background(), "c-1", time.Time{}, 0); err != nil {
		t.Errorf("nil repo 零参数不应报错，got %v", err)
	}
}
