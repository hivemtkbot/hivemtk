// 拆分自 unified_inbox.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import "context"

func (s *UnifiedInboxService) GetCustomerByUnifiedID(ctx context.Context, unifiedID string) *OneIDCustomerLite {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.customerByUnifiedID[unifiedID]; ok {
		cp := *c
		return &cp
	}
	return nil
}

// channelLabel 渠道展示标签
func (s *UnifiedInboxService) channelLabel(ctx context.Context, c InboxChannel) string {
	switch c {
	case InboxChannelWeChat:
		return "微信"
	case InboxChannelDouyin:
		return "抖音"
	case InboxChannelXiaohongshu:
		return "小红书"
	case InboxChannelEmail:
		return "邮件"
	case InboxChannelWeb:
		return "网页"
	}
	return string(c)
}

func inboxTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (indexFold(s, sub) >= 0)
}

func indexFold(s, sub string) int {
	// 简化的大小写不敏感包含
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
