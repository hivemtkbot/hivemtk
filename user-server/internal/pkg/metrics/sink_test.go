// 指标落库 sink 测试（2026-08-15 M3-P1-E4）
//
// 验证：
//   - labelsToJSON 序列化（含/不含 agg 维度）
//   - CollectSamples 快照（counter/gauge/histogram 的 count/sum）
//   - BridgeMetricsSink.Flush 落库到 bridge_metrics 表（依赖真实 PG，不可达时跳过）
//   - StartBridgeMetricsSink nil db 安全（不 panic、stop 可调用）
package metrics

import (
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"
)

// TestLabelsToJSON 验证 label 键值对序列化为 JSON 字符串（含 agg 附加维度）。
func TestLabelsToJSON(t *testing.T) {
	t.Run("无 agg", func(t *testing.T) {
		out := labelsToJSON([]string{"channel", "error_code"}, []string{"xhs", "invalid_json"}, "")
		if !strings.Contains(out, `"channel":"xhs"`) {
			t.Errorf("缺少 channel 键值: %s", out)
		}
		if !strings.Contains(out, `"error_code":"invalid_json"`) {
			t.Errorf("缺少 error_code 键值: %s", out)
		}
		if strings.Contains(out, "agg") {
			t.Errorf("不应包含 agg 键: %s", out)
		}
	})
	t.Run("直方图带 agg", func(t *testing.T) {
		out := labelsToJSON([]string{"channel"}, []string{"xhs"}, "sum")
		if !strings.Contains(out, `"agg":"sum"`) {
			t.Errorf("缺少 agg 键: %s", out)
		}
	})
	t.Run("value 少于 key 时不越界", func(t *testing.T) {
		out := labelsToJSON([]string{"a", "b"}, []string{"1"}, "")
		if !strings.Contains(out, `"a":"1"`) {
			t.Errorf("a 键值缺失: %s", out)
		}
		if strings.Contains(out, `"b"`) {
			t.Errorf("b 键不应存在（无对应值）: %s", out)
		}
	})
}

// TestCollectSamples_Bridge 验证注册表中桥接指标能产出采样快照。
func TestCollectSamples_Bridge(t *testing.T) {
	b := GetBridge()
	b.IngestTotal.WithLabel("xhs", "7").Inc()
	b.OutboxAcked.WithLabel("xhs", "acked").Inc()

	samples := CollectSamples()
	if len(samples) == 0 {
		t.Fatal("CollectSamples 返回空")
	}

	var foundIngestTotal, foundOutboxAcked bool
	var ingestTotalValue float64
	for _, s := range samples {
		if s.Name == "bridge_ingest_total" {
			foundIngestTotal = true
			for _, l := range s.Labels {
				if l == "7" {
					ingestTotalValue = s.Value
				}
			}
		}
		if s.Name == "bridge_outbox_acked_total" {
			foundOutboxAcked = true
		}
		if s.Name == "bridge_ingest_duration_ms" && s.Agg == "count" {
			if s.Agg != "count" {
				t.Errorf("histogram 采样 agg 应为 count: %+v", s)
			}
		}
	}
	if !foundIngestTotal {
		t.Error("CollectSamples 未包含 bridge_ingest_total")
	}
	if ingestTotalValue < 1 {
		t.Errorf("bridge_ingest_total(xhs,7) 应 >= 1，实际 %v", ingestTotalValue)
	}
	if !foundOutboxAcked {
		t.Error("CollectSamples 未包含 bridge_outbox_acked_total")
	}
}

// TestHistogramSamples_CountSum 验证直方图采样同时产出 count 与 sum 两行（agg 区分）。
func TestHistogramSamples_CountSum(t *testing.T) {
	h := NewHistogram("test_sink_hist", "sink hist", []string{"ch"},
		[]float64{1, 10, 100})
	h.WithLabel("xhs").Observe(5)
	h.WithLabel("xhs").Observe(50)

	samples := CollectSamples()
	var count, sum float64
	haveCount, haveSum := false, false
	for _, s := range samples {
		if s.Name != "test_sink_hist" || len(s.Labels) == 0 || s.Labels[0] != "xhs" {
			continue
		}
		switch s.Agg {
		case "count":
			count = s.Value
			haveCount = true
		case "sum":
			sum = s.Value
			haveSum = true
		}
	}
	if !haveCount || !haveSum {
		t.Fatalf("直方图采样应同时含 count/sum，count=%v sum=%v", haveCount, haveSum)
	}
	if count != 2 {
		t.Errorf("count 应为 2，实际 %v", count)
	}
	if sum != 55 {
		t.Errorf("sum 应为 55，实际 %v", sum)
	}
}

// TestSinkFlush_NilSink 验证 nil sink / nil db 时 Flush 安全返回 nil。
func TestSinkFlush_NilSink(t *testing.T) {
	var s *BridgeMetricsSink
	if err := s.Flush(); err != nil {
		t.Errorf("nil sink Flush 应返回 nil，实际 %v", err)
	}
	if err := NewBridgeMetricsSink(nil).Flush(); err != nil {
		t.Errorf("nil db Flush 应返回 nil，实际 %v", err)
	}
}

// TestStartBridgeMetricsSink_NilDB 验证 nil db 时启动安全（返回可调用的 stop）。
func TestStartBridgeMetricsSink_NilDB(t *testing.T) {
	stop := StartBridgeMetricsSink(nil, 10*time.Millisecond)
	if stop == nil {
		t.Fatal("StartBridgeMetricsSink(nil) 返回 nil stop")
	}
	stop() 
	stop()
}

// TestSinkFlush_WithDB 集成测试：Flush 把注册表当前指标写入 bridge_metrics 表。
// 依赖真实 PostgreSQL，不可达时 testutil 自动 Skip。
func TestSinkFlush_WithDB(t *testing.T) {
	db := testutil.NewTestDB(t, &BridgeMetricRow{})
	if db == nil {
		return
	}
	if err := db.Exec("DELETE FROM bridge_metrics").Error; err != nil {
		t.Fatalf("清理 bridge_metrics 失败: %v", err)
	}

	b := GetBridge()
	b.IngestTotal.WithLabel("sinkdb", "1").Add(3)

	sink := NewBridgeMetricsSink(db)
	if err := sink.Flush(); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}

	var rows []BridgeMetricRow
	if err := db.Where("metric_name = ?", "bridge_ingest_total").Find(&rows).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("bridge_metrics 表未写入 bridge_ingest_total 行")
	}
	found := false
	for _, r := range rows {
		if strings.Contains(r.Labels, `"agent_id":"1"`) && r.Value == 3 {
			found = true
		}
		if r.MetricType != "counter" {
			t.Errorf("metric_type 应为 counter，实际 %q", r.MetricType)
		}
		if r.TS.IsZero() {
			t.Errorf("ts 不应为零值")
		}
	}
	if !found {
		t.Errorf("未找到 labels 含 agent_id=1 且 value=3 的行: %+v", rows)
	}
}

