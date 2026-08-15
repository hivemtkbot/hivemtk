package websocket

import (
	"sync"
	"testing"
)

// TestNextSeq_Monotonic 序号单调递增
func TestNextSeq_Monotonic(t *testing.T) {
	const N = 1000
	seqs := make([]uint64, N)
	for i := 0; i < N; i++ {
		seqs[i] = NextSeq()
	}
	for i := 1; i < N; i++ {
		if seqs[i] <= seqs[i-1] {
			t.Errorf("seq not monotonic: seqs[%d]=%d <= seqs[%d]=%d", i, seqs[i], i-1, seqs[i-1])
		}
	}
}

// TestNextSeq_NeverZero 序号永不为 0（0 保留为未分配哨兵）
func TestNextSeq_NeverZero(t *testing.T) {
	for i := 0; i < 100; i++ {
		if s := NextSeq(); s == 0 {
			t.Fatal("NextSeq returned 0")
		}
	}
}

// TestNextSeq_ConcurrentSafe 并发安全：100 协程各取 1000 seq，结果唯一
func TestNextSeq_ConcurrentSafe(t *testing.T) {
	const (
		Goroutines = 100
		PerG       = 1000
	)
	seqs := make(chan uint64, Goroutines*PerG)
	var wg sync.WaitGroup
	wg.Add(Goroutines)
	for g := 0; g < Goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < PerG; i++ {
				seqs <- NextSeq()
			}
		}()
	}
	wg.Wait()
	close(seqs)

	seen := make(map[uint64]struct{}, Goroutines*PerG)
	for s := range seqs {
		if _, dup := seen[s]; dup {
			t.Fatalf("duplicate seq: %d", s)
		}
		seen[s] = struct{}{}
	}
	if len(seen) != Goroutines*PerG {
		t.Errorf("expected %d unique seqs, got %d", Goroutines*PerG, len(seen))
	}
}

// TestPeekSeq_DoesNotAdvance PeekSeq 只读不递增
func TestPeekSeq_DoesNotAdvance(t *testing.T) {
	before := PeekSeq()
	_, _ = PeekSeq(), PeekSeq()
	after := PeekSeq()
	if before != after {
		t.Errorf("PeekSeq advanced: before=%d after=%d", before, after)
	}
}

