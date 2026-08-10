package auto_reply_integration

import (
	"time"

	ragcustomerservice "hivemtk-user/internal/aiagent/rag/customer_service"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
)

// NewDefaultAutoReplyIntegrationService 创建默认的自动回复集成服务
func NewDefaultAutoReplyIntegrationService(
	ragService ragcustomerservice.RagCustomerService,
	retrievalService ragretrieval.RagRetrievalService,
) *AutoReplyIntegrationServiceImpl {

	// 创建默认规则匹配器
	defaultRuleMatcher := NewDefaultRuleBasedMatcher()

	// 设置默认超时时间
	defaultTimeout := 15 * time.Second

	// 创建并返回服务实例
	return NewAutoReplyIntegrationService(
		ragService,
		retrievalService,
		defaultRuleMatcher, // 传递整个DefaultRuleBasedMatcher实例，它实现了RuleBasedMatcher接口
		defaultTimeout,
	)
}
