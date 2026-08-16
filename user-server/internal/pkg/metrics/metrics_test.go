// 指标包基础测试（2026-08-15 M3-P1-E3）
//
// 验证：
//   - Counter 增 / 加
//   - Gauge 增减
//   - Histogram Observe
//   - 标签隔离
//   - 指标文本格式输出
//   - 全局注册表 / Handler
package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCounter_BasicInc(t *testing.T) {
	c := NewCounter("test_counter_inc", "Test counter", []string{"label"})
	c.WithLabel("a").Inc()
	c.WithLabel("a").Inc()
	c.WithLabel("b").Inc()

	if c.WithLabel("a").Value() != 2 {
		t.Fatalf("expected counter a=2, got %d", c.WithLabel("a").Value())
	}
	if c.WithLabel("b").Value() != 1 {
		t.Fatalf("expected counter b=1, got %d", c.WithLabel("b").Value())
	}
}

func TestCounter_Add(t *testing.T) {
	c := NewCounter("test_counter_add", "Test counter", []string{})
	c.WithLabel().Add(100)
	c.WithLabel().Add(50)
	if c.WithLabel().Value() != 150 {
		t.Fatalf("expected 150, got %d", c.WithLabel().Value())
	}
}

func TestGauge_SetIncDec(t *testing.T) {
	g := NewGauge("test_gauge_basic", "Test gauge", []string{})
	g.WithLabel().Set(100)
	if g.WithLabel().Value() != 100 {
		t.Fatalf("expected 100, got %d", g.WithLabel().Value())
	}
	g.WithLabel().Inc()
	g.WithLabel().Inc()
	g.WithLabel().Dec()
	if g.WithLabel().Value() != 101 {
		t.Fatalf("expected 101, got %d", g.WithLabel().Value())
	}
}

func TestHistogram_Observe(t *testing.T) {
	h := NewHistogram("test_histogram_basic", "Test histogram", []string{"l"},
		[]float64{1, 5, 10})
	h.WithLabel("a").Observe(0.5)  
	h.WithLabel("a").Observe(3)    
	h.WithLabel("a").Observe(7)    
	h.WithLabel("a").Observe(20)   
	h.WithLabel("a").Observe(20)
}

func TestMetricsOutput_Format(t *testing.T) {
	c := NewCounter("test_prom_output_counter", "Test output", []string{"foo"})
	c.WithLabel("bar").Inc()
	c.WithLabel("bar").Add(5)

	var sb strings.Builder
	c.Write(&sb)
	out := sb.String()

	if !strings.Contains(out, "# HELP test_prom_output_counter Test output") {
		t.Errorf("missing HELP: %s", out)
	}
	if !strings.Contains(out, "# TYPE test_prom_output_counter counter") {
		t.Errorf("missing TYPE: %s", out)
	}
	if !strings.Contains(out, `test_prom_output_counter{foo="bar"} 6`) {
		t.Errorf("missing value: %s", out)
	}
}

func TestHandler_HTTP(t *testing.T) {
	c := NewCounter("test_handler_counter", "Test handler", []string{"x"})
	c.WithLabel("y").Inc()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain, got %s", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "test_handler_counter") {
		t.Errorf("missing test_handler_counter in body: %s", body)
	}
}

func TestConcurrentSafety(t *testing.T) {
	c := NewCounter("test_concurrent_counter", "Test concurrent", []string{})
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				c.WithLabel().Inc()
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	if c.WithLabel().Value() != 10000 {
		t.Fatalf("expected 10000, got %d", c.WithLabel().Value())
	}
}

func TestLabelValidation(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for wrong label count")
		}
	}()
	c := NewCounter("test_label_validation", "Test", []string{"a", "b"})
	c.WithLabel("only_one") 
}

func TestGetBridge_AllMetrics(t *testing.T) {
	b := GetBridge()
	if b == nil {
		t.Fatal("GetBridge returned nil")
	}
	expected := []string{
		"bridge_ingest_total",
		"bridge_ingest_errors_total",
		"bridge_ingest_duration_ms",
		"bridge_outbox_fetched_total",
		"bridge_outbox_acked_total",
		"bridge_ack_duration_ms",
		"bridge_circuit_breaker_state",
		"bridge_pending_ack_size",
		"bridge_pending_dead_letters",
		"bridge_dlq_total",
		"bridge_emergency_stop",
		"bridge_idempotency_hits_total",
		"bridge_pii_redactions_total",
	}
	for _, name := range expected {
		if Get(name) == nil {
			t.Errorf("metric %s not registered", name)
		}
	}
}


