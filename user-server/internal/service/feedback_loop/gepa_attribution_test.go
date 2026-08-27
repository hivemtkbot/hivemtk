package feedbackloop

import (
	"math"
	"sync"
	"testing"
)

const epsGepa = 1e-9

func nearGepa(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > epsGepa {
		t.Fatalf("%s = %.6f, want %.6f", name, got, want)
	}
}

// 聚合与排序手工验证：
// B 3 次失败排第一；C/A 同为 2 次，C 的 AvgDelta(-0.8) < A(-0.2)，C 排前。
func TestFailureLedgerTopLessons(t *testing.T) {
	ledger := NewFailureLedger()
	ledger.RecordFailure(FailureAttribution{LineageID: "A", SampleScore: 0.2, JudgeReason: "过度承诺", Decision: "gate_failed", Delta: -0.5})
	ledger.RecordFailure(FailureAttribution{LineageID: "A", SampleScore: 0.4, JudgeReason: "过度承诺", Decision: "rolled_back", Delta: 0.1})
	ledger.RecordFailure(FailureAttribution{LineageID: "B", SampleScore: 0.6, JudgeReason: "信息缺失", Decision: "gate_failed", Delta: -0.2})
	ledger.RecordFailure(FailureAttribution{LineageID: "B", SampleScore: 0.5, JudgeReason: "合规风险", Decision: "gate_failed", Delta: -0.3})
	ledger.RecordFailure(FailureAttribution{LineageID: "B", SampleScore: 0.7, JudgeReason: "信息缺失", Decision: "rolled_back", Delta: -0.1})
	ledger.RecordFailure(FailureAttribution{LineageID: "C", SampleScore: 0.1, JudgeReason: "误导话术", Decision: "manual_review", Delta: -1.0})
	ledger.RecordFailure(FailureAttribution{LineageID: "C", SampleScore: 0.3, JudgeReason: "误导话术", Decision: "gate_failed", Delta: -0.6})

	lessons := ledger.TopLessons(10)
	if len(lessons) != 3 {
		t.Fatalf("lesson 数=%d, want 3", len(lessons))
	}
	if lessons[0].LineageID != "B" || lessons[1].LineageID != "C" || lessons[2].LineageID != "A" {
		t.Fatalf("排序错误: %+v", lessons)
	}

	b := lessons[0]
	if b.Failures != 3 {
		t.Fatalf("B 失败数=%d, want 3", b.Failures)
	}
	nearGepa(t, "B.AvgScore", b.AvgSampleScore, (0.6+0.5+0.7)/3)
	nearGepa(t, "B.AvgDelta", b.AvgDelta, -0.2)
	if b.TopReason != "信息缺失" || b.TopDecision != "gate_failed" {
		t.Fatalf("B 高频项错误: %+v", b)
	}

	c := lessons[1]
	nearGepa(t, "C.AvgDelta", c.AvgDelta, -0.8)
	if c.TopReason != "误导话术" || c.TopDecision != "gate_failed" {
		t.Fatalf("C 高频项错误（1:1 并列应取字典序 gate_failed）: %+v", c)
	}

	a := lessons[2]
	nearGepa(t, "A.AvgScore", a.AvgSampleScore, 0.3)
	if a.TopReason != "过度承诺" || a.TopDecision != "gate_failed" {
		t.Fatalf("A 高频项错误: %+v", a)
	}

	if got := ledger.TopLessons(2); len(got) != 2 || got[0].LineageID != "B" || got[1].LineageID != "C" {
		t.Fatalf("截断错误: %+v", got)
	}
	if got := ledger.TopLessons(0); len(got) != 0 {
		t.Fatalf("n=0 应为空: %+v", got)
	}
}

// 空台账返回空；零值结构体懒初始化后可直接记账
func TestFailureLedgerEmptyAndZeroValue(t *testing.T) {
	if got := NewFailureLedger().TopLessons(3); len(got) != 0 {
		t.Fatalf("空台账应返回空: %+v", got)
	}
	var zero FailureLedger
	zero.RecordFailure(FailureAttribution{LineageID: "Z", JudgeReason: "r", Decision: "d"})
	lessons := zero.TopLessons(1)
	if len(lessons) != 1 || lessons[0].LineageID != "Z" || lessons[0].Failures != 1 {
		t.Fatalf("零值结构体记账失败: %+v", lessons)
	}
}

// 并发记账计数正确、无死锁
func TestFailureLedgerConcurrentRecord(t *testing.T) {
	ledger := NewFailureLedger()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ledger.RecordFailure(FailureAttribution{LineageID: "S", SampleScore: 1, Delta: -1})
		}()
	}
	wg.Wait()
	lessons := ledger.TopLessons(5)
	if len(lessons) != 1 || lessons[0].Failures != 50 {
		t.Fatalf("并发记账结果错误: %+v", lessons)
	}
}
