package metrics

// metrics_test.go 置信度/拟人度/反馈学习 业务指标埋点测试
//
// 验证 GlobalMetrics 各向量类型的 Inc/Observe/Range 行为正确，
// 避免 import cycle 后的回归。

import "testing"

// TestCounterVec 测试计数器
func TestCounterVec(t *testing.T) {
	cv := &CounterVec{values: make(map[string]uint64)}

	cv.Inc("a")
	cv.Inc("a")
	cv.Inc("b")

	if cv.Value("a") != 2 {
		t.Errorf("expected a=2, got %d", cv.Value("a"))
	}
	if cv.Value("b") != 1 {
		t.Errorf("expected b=1, got %d", cv.Value("b"))
	}

	total := 0
	cv.Range(func(labels string, value uint64) {
		total += int(value)
	})
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
}

// TestGauge 测试仪表盘
func TestGauge(t *testing.T) {
	g := &Gauge{}

	g.Inc()
	g.Inc()
	g.Inc()
	if g.Value() != 3 {
		t.Errorf("expected 3, got %d", g.Value())
	}
	g.Dec()
	if g.Value() != 2 {
		t.Errorf("expected 2 after Dec, got %d", g.Value())
	}
	g.Set(100)
	if g.Value() != 100 {
		t.Errorf("expected 100 after Set, got %d", g.Value())
	}
}

// TestSummaryVec 测试摘要向量
func TestSummaryVec(t *testing.T) {
	sv := &SummaryVec{}

	sv.Observe("k", 1.0)
	sv.Observe("k", 2.0)
	sv.Observe("k", 3.0)

	seen := false
	sv.Range(func(labels string, sum float64, count uint64) {
		if labels == "k" {
			seen = true
			if sum != 6.0 {
				t.Errorf("expected sum=6, got %f", sum)
			}
			if count != 3 {
				t.Errorf("expected count=3, got %d", count)
			}
		}
	})
	if !seen {
		t.Error("expected to see label k")
	}
}

// TestGlobalMetricsInitialization 测试全局指标初始化
func TestGlobalMetricsInitialization(t *testing.T) {
	m := GlobalMetrics
	if m == nil {
		t.Fatal("GlobalMetrics is nil")
	}
	if m.ConfidenceScoredTotal == nil {
		t.Error("ConfidenceScoredTotal not initialized")
	}
	if m.HumanizeScoredTotal == nil {
		t.Error("HumanizeScoredTotal not initialized")
	}
	if m.FeedbackEventsTotal == nil {
		t.Error("FeedbackEventsTotal not initialized")
	}
	if m.FeedbackBanditRewardsTotal == nil {
		t.Error("FeedbackBanditRewardsTotal not initialized")
	}
}

// TestTuningMetricFunctions 测试 置信度/拟人度/反馈学习 业务埋点函数
func TestTuningMetricFunctions(t *testing.T) {
	// 置信度
	RecordConfidenceScored("test_scenario")
	RecordConfidenceDecision("test_scenario", "transfer")
	RecordConfidenceDecision("test_scenario", "auto_reply")

	if GlobalMetrics.ConfidenceScoredTotal.Value("test_scenario") != 1 {
		t.Errorf("ConfidenceScored not recorded")
	}
	if GlobalMetrics.ConfidenceTransferTotal.Value("test_scenario") != 1 {
		t.Errorf("ConfidenceTransfer not recorded")
	}
	if GlobalMetrics.ConfidenceAutoReplyTotal.Value("test_scenario") != 1 {
		t.Errorf("ConfidenceAutoReply not recorded")
	}

	// 拟人度
	RecordHumanizeScored("rule", "full")
	RecordHumanizeRegenerate("rule")
	ObserveHumanizeScore("rule", 0.85)

	if GlobalMetrics.HumanizeScoredTotal.Value("rule|full") != 1 {
		t.Errorf("HumanizeScored not recorded")
	}
	if GlobalMetrics.HumanizeRegenerateTotal.Value("rule") != 1 {
		t.Errorf("HumanizeRegenerate not recorded")
	}

	// 反馈学习
	RecordFeedbackEvent("explicit", "like")
	RecordBanditSample("arm1")
	ObserveBanditReward("arm1", 0.9)
	RecordPromptCandidate("scenario1")

	if GlobalMetrics.FeedbackEventsTotal.Value("explicit|like") != 1 {
		t.Errorf("FeedbackEvents not recorded")
	}
	if GlobalMetrics.FeedbackBanditSamplesTotal.Value("arm1") != 1 {
		t.Errorf("BanditSample not recorded")
	}
	if GlobalMetrics.FeedbackPromptCandidatesTotal.Value("scenario1") != 1 {
		t.Errorf("PromptCandidate not recorded")
	}
}

// TestHistogramVecNoMemoryLeak 验证 HistogramVec 不再保留 buckets 切片
//
// 回归测试：原实现将所有观测值 append 到 buckets[labels] 切片中且从不释放，
// 在持续高流量下会导致 OOM。修复后 HistogramVec 仅保留 sum/count 聚合。
func TestHistogramVecNoMemoryLeak(t *testing.T) {
	h := &HistogramVec{}
	// 模拟大量观测
	for i := 0; i < 10000; i++ {
		h.Observe("leak_test", float64(i)*0.001)
	}

	var sum float64
	var count uint64
	h.Range(func(labels string, s float64, c uint64) {
		if labels == "leak_test" {
			sum = s
			count = c
		}
	})

	if count != 10000 {
		t.Errorf("expected count=10000, got %d", count)
	}
	// sum = 0.001 + 0.002 + ... + 9.999 = 0.001 * (0+1+2+...+9999) = 0.001 * 49995000 = 49995
	expectedSum := 0.001 * (9999.0 * 10000.0 / 2.0)
	if (sum-expectedSum) > 1e-6 || (expectedSum-sum) > 1e-6 {
		t.Errorf("expected sum≈%f, got %f", expectedSum, sum)
	}
}

// TestRangeDoesNotBlockWrites 验证 Range 不会长时间阻塞 Inc/Observe
//
// 回归测试：原实现 Range 在持有 RLock 期间执行用户回调，
// 如果回调耗时（如 /metrics 端点字符串拼接），会阻塞所有写操作。
// 修复后 Range 先复制快照再执行回调，回调期间可安全写入。
func TestRangeDoesNotBlockWrites(t *testing.T) {
	cv := &CounterVec{}
	for i := 0; i < 100; i++ {
		cv.Inc("a")
	}

	// 在 Range 回调中执行写操作（验证快照语义：不会死锁，不要求看到新写入的值）
	cv.Range(func(labels string, value uint64) {
		cv.Inc("b") // 在读循环中写
	})

	// "a" 有 100 次自增；Range 只对 "a" 触发一次回调，所以 "b" 应该 = 1
	if cv.Value("b") != 1 {
		t.Errorf("expected b=1 (one Range iteration), got %d", cv.Value("b"))
	}
	if cv.Value("a") != 100 {
		t.Errorf("expected a=100, got %d", cv.Value("a"))
	}
}
