package channelgw

import (
	"testing"

	"hivemtk-user/internal/model"
)

// TestRegistry_RegisterAndQuery 注册/查询/覆盖/并发读基础行为。
func TestRegistry_RegisterAndQuery(t *testing.T) {
	r := NewRegistry()
	r.Register(ChannelSpec{Name: "demo", Transports: []Transport{TransportHTTP}, Label: "演示"})

	if !r.IsChannel("demo") {
		t.Error("IsChannel(demo) = false, want true")
	}
	if r.IsChannel("unknown") {
		t.Error("未注册渠道不应命中")
	}
	if !r.Supports("demo", TransportHTTP) {
		t.Error("demo 应支持 http")
	}
	if r.Supports("demo", TransportWebSocket) {
		t.Error("demo 未声明 websocket，不应支持")
	}
	if r.Supports("unknown", TransportHTTP) {
		t.Error("未注册渠道 Supports 应为 false")
	}

	spec, ok := r.Spec("demo")
	if !ok || spec.Label != "演示" {
		t.Errorf("Spec 查询错误: %+v ok=%v", spec, ok)
	}
	if names := r.Names(); len(names) != 1 || names[0] != "demo" {
		t.Errorf("Names = %v, want [demo]", names)
	}

	r.Register(ChannelSpec{Name: "demo", Transports: []Transport{TransportHTTP, TransportWebSocket}})
	if !r.Supports("demo", TransportWebSocket) {
		t.Error("同名覆盖后应支持 websocket")
	}

	r.Register(ChannelSpec{Name: ""})
	if r.IsChannel("") {
		t.Error("空名注册应被忽略")
	}
}

// TestDefaultRegistry 默认注册表覆盖 5 大社交渠道，且均支持 HTTP + WebSocket 双传输。
func TestDefaultRegistry(t *testing.T) {
	want := []string{
		model.ChannelDouyin,
		model.ChannelXHS,
		model.ChannelKuaishou,
		model.ChannelXianyu,
		model.ChannelTikTok,
	}
	for _, ch := range want {
		if !Default.IsChannel(ch) {
			t.Errorf("Default 缺渠道 %s", ch)
			continue
		}
		if !Default.Supports(ch, TransportHTTP) {
			t.Errorf("渠道 %s 应支持 http", ch)
		}
		if !Default.Supports(ch, TransportWebSocket) {
			t.Errorf("渠道 %s 应支持 websocket", ch)
		}
	}
	if len(Default.Names()) != len(want) {
		t.Errorf("Default 渠道数 = %d, want %d", len(Default.Names()), len(want))
	}
}

