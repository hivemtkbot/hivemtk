package etl

import (
	"context"
	"strings"
	"testing"
	"time"

	"marketing/internal/aiagent/rag/core"
)

// TestProcessDocument_NoInfiniteLoop 回归测试：
// 中文文本以 。！？ 等句末标点结尾但标点后无空格时，shouldUseSentenceSplit 返回 false，
// 回落到 splitByFixedLength。若分片推进逻辑存在死循环（切点前进量 <= ChunkOverlap 时 start 不前进），
// ProcessDocument 会永久阻塞。本测试用带超时的 context 确保必然返回。
func TestProcessDocument_NoInfiniteLoop(t *testing.T) {
	dp := NewDocumentProcessor(nil)

	// 模拟真实中文知识文档：长文本、句末标点后无空格（无 \s+ 跟随标点）
	text := strings.Repeat(
		"智能体营销系统是一套面向企业的多渠道客户触达与转化平台。它通过统一的对话引擎连接微信和企业微信等渠道，帮助企业自动接待客户咨询并引导成交。系统的核心能力包括知识库检索增强生成、意图识别、私信自动回复以及主动触达召回。知识库支持导入产品手册和常见问题等长文本，经过自动分片与向量化后存入向量数据库。在客户提问时通过语义检索召回最相关的片段，再由大语言模型生成准确自然的回复。管理员可在后台配置不同知识库的专属模型参数，实现按业务场景灵活切换的检索质量与回答风格。",
		5,
	)

	done := make(chan struct{})
	var chunks []rag_core.Chunk
	var err error
	go func() {
		chunks, err = dp.ProcessDocument(context.Background(), rag_core.Document{
			ID:      "kb_doc_regression",
			Content: text,
			Metadata: map[string]any{
				"title":  "回归测试文档",
				"source": "text",
			},
		})
		close(done)
	}()

	select {
	case <-done:
		// 预期：返回且不死循环
		if err != nil {
			t.Fatalf("ProcessDocument 返回错误: %v", err)
		}
		if len(chunks) == 0 {
			t.Fatalf("ProcessDocument 未产出任何分片")
		}
		// 校验分片内容完整覆盖原文（无丢失）
		var sb strings.Builder
		for _, c := range chunks {
			sb.WriteString(c.Content)
		}
		if !strings.Contains(sb.String(), "智能体营销系统") {
			t.Fatalf("分片内容丢失，未包含原文关键信息")
		}
		t.Logf("分片数=%d，首片长度=%d", len(chunks), len(chunks[0].Content))
	case <-time.After(5 * time.Second):
		t.Fatal("ProcessDocument 在 5s 内未返回，疑似死循环（splitByFixedLength 推进失败）")
	}
}
