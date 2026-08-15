package ragretrieval

import (
	"context"
	"errors"
	"testing"
)

func TestRAGThreeTierAdapter_Basic(t *testing.T) {
	v := &mockVectorizer{dim: 8}
	l2 := &mockIndex{}
	l2.chunks = map[string][]Chunk{
		"kb1": {{ID: "c1", Content: "hello", Score: 0.9}},
	}
	svc := NewRAGThreeTierService(nil, v, l2, nil, nil, 10)
	adapter := NewRAGThreeTierAdapter(svc)

	res, err := adapter.Search(context.Background(), "kb1", "hello", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res == nil {
		t.Fatal("expected result")
	}
	if res.Source != string(TierL2WarmIndex) {
		t.Errorf("expected L2 source, got %s", res.Source)
	}
	if len(res.Chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(res.Chunks))
	}
}

func TestRAGThreeTierAdapter_Stats(t *testing.T) {
	v := &mockVectorizer{dim: 8}
	l2 := &mockIndex{}
	l2.chunks = map[string][]Chunk{"kb1": {{ID: "c1", Score: 0.9}}}
	svc := NewRAGThreeTierService(nil, v, l2, nil, nil, 10)
	adapter := NewRAGThreeTierAdapter(svc)

	adapter.Search(context.Background(), "kb1", "hi", 5)
	adapter.Search(context.Background(), "kb1", "hi", 5) 
	stats := adapter.Stats()
	if stats.Total < 1 {
		t.Errorf("expected total >= 1, got %d", stats.Total)
	}
}

func TestRAGThreeTierAdapter_NilService(t *testing.T) {
	adapter := &RAGThreeTierAdapter{svc: nil}
	res, err := adapter.Search(context.Background(), "kb1", "hi", 5)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if res != nil {
		t.Errorf("expected nil result, got %+v", res)
	}
	stats := adapter.Stats()
	if stats.Total != 0 {
		t.Errorf("expected zero stats, got %+v", stats)
	}
}

func TestRAGThreeTierAdapter_ErrorPropagation(t *testing.T) {
	v := &mockVectorizer{dim: 8}
	l2 := &mockIndex{err: errors.New("backend down")}
	svc := NewRAGThreeTierService(nil, v, l2, nil, nil, 10)
	adapter := NewRAGThreeTierAdapter(svc)
	res, err := adapter.Search(context.Background(), "kb1", "hi", 5)
	if err != nil {
		t.Errorf("expected graceful degrade, got error: %v", err)
	}
	if res == nil {
		t.Fatal("expected result")
	}
	if res.Source != "" {
		t.Errorf("expected empty source, got %s", res.Source)
	}
}

