package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// smsUnsubscribeKeywords 短信退订关键词列表
//
// 合规依据：《通信短消息服务管理规定》第十八条：用户回复"TD/退订/T退/取消/N/Q"等
// 关键词视为明确表示拒绝接收商业性短消息，必须停止发送。
// 关键词匹配规则：
//   - 大小写不敏感（N/n/Q/q 等价）
//   - 全角/半角兼容
//   - 关键词前后允许空格、标点（用户回复可能为 "TD, 退订" 等）
//   - 匹配到任意一个关键词即视为退订请求
var smsUnsubscribeKeywords = []string{
	"TD", "td", "Td", "tD",
	"退订", "退定", "取消订阅", "拒绝接收",
	"T退", "t退",
	"取消", "不想接收", "停止",
	"N", "n",
	"Q", "q",
	"0",
	"Unsubscribe", "unsubscribe", "STOP", "stop",
}

// SmsUnsubscribeService 短信退订服务
//
// 合规要点：
//   - UnsubscribePhone：记录退订请求，关键词命中留痕
//   - IsUnsubscribed：发送前必须调用，命中则跳过发送
//   - ProcessUnsubscribeReply：解析上行短信，识别退订关键词
//   - ResubscribePhone：允许用户重新订阅（合规要求）
type SmsUnsubscribeService struct {
	repo repository.SmsUnsubscribeRepository
}

// NewSmsUnsubscribeService 创建短信退订服务
func NewSmsUnsubscribeService(repo repository.SmsUnsubscribeRepository) *SmsUnsubscribeService {
	if repo == nil {
		repo = repository.NewSmsUnsubscribeRepository(nil)
	}
	return &SmsUnsubscribeService{repo: repo}
}

// UnsubscribePhone 记录手机号退订请求
// 重复退订幂等：若已存在退订记录，更新退订时间和原因
func (s *SmsUnsubscribeService) UnsubscribePhone(ctx context.Context, phone, reason, msgID, keyword string) error {
	phone = NormalizePhone(phone)
	if phone == "" {
		return errors.New("phone 不能为空")
	}

	existing, err := s.repo.GetByPhone(ctx, phone)
	if err != nil {
		logger.Errorf("查询短信退订记录失败 phone=%s: %v", phone, err)
		return err
	}

	now := time.Now()
	if existing != nil {
		existing.Reason = reason
		existing.SourceMessageID = msgID
		existing.KeywordMatched = keyword
		existing.UnsubscribedAt = now
		return s.repo.Update(ctx, existing)
	}

	record := &model.SmsUnsubscribe{
		Phone:			phone,
		Reason:			reason,
		UnsubscribedAt:		now,
		SourceMessageID:	msgID,
		KeywordMatched:		keyword,
	}
	return s.repo.Create(ctx, record)
}

// IsUnsubscribed 检查手机号是否已退订（发送前必须调用）
func (s *SmsUnsubscribeService) IsUnsubscribed(ctx context.Context, phone string) bool {
	phone = NormalizePhone(phone)
	if phone == "" {
		return false
	}
	exists, err := s.repo.ExistsByPhone(ctx, phone)
	if err != nil {
		logger.Errorf("查询短信退订状态失败 phone=%s: %v", phone, err)
		return false
	}
	return exists
}

// ResubscribePhone 允许用户重新订阅（合规要求）
func (s *SmsUnsubscribeService) ResubscribePhone(ctx context.Context, phone string) error {
	phone = NormalizePhone(phone)
	if phone == "" {
		return errors.New("phone 不能为空")
	}
	return s.repo.DeleteByPhone(ctx, phone)
}

// ProcessUnsubscribeReply 处理上行短信回复
//
// 解析用户回复内容，若命中退订关键词则自动加入退订名单
// 返回值 matchedKeyword：命中的关键词（未命中时为空字符串）
func (s *SmsUnsubscribeService) ProcessUnsubscribeReply(ctx context.Context, phone, replyContent, msgID string) (matchedKeyword string, err error) {
	phone = NormalizePhone(phone)
	if phone == "" {
		return "", errors.New("phone 不能为空")
	}
	if replyContent == "" {
		return "", errors.New("reply_content 不能为空")
	}

	keyword := MatchUnsubscribeKeyword(replyContent)
	if keyword == "" {
		return "", nil
	}

	reason := "用户回复关键词：" + replyContent
	if err := s.UnsubscribePhone(ctx, phone, reason, msgID, keyword); err != nil {
		return keyword, err
	}
	return keyword, nil
}

// ListUnsubscribes 分页查询退订名单
func (s *SmsUnsubscribeService) ListUnsubscribes(ctx context.Context, page, limit int, keyword string) ([]*model.SmsUnsubscribe, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 20
	}
	return s.repo.List(ctx, page, limit, keyword)
}

// ListAllUnsubscribes 查询全部退订名单（导出使用）
func (s *SmsUnsubscribeService) ListAllUnsubscribes(ctx context.Context,) ([]*model.SmsUnsubscribe, error) {
	return s.repo.ListAll(ctx)
}

// MatchUnsubscribeKeyword 检查回复内容是否包含退订关键词
//
// 返回命中的关键词（原始大小写），未命中返回空字符串
// 匹配策略：
//  1. 完全匹配（去除前后空格后）优先，避免误判"TD-RFID"等
//  2. 单字符关键词（N/Q/0/TD）必须独立词，避免误判"NT999"
//  3. 中文关键词支持子串匹配（"TD退订"）
func MatchUnsubscribeKeyword(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	// 1. 完全匹配（最高优先级）
	for _, kw := range smsUnsubscribeKeywords {
		if strings.EqualFold(content, kw) {
			return kw
		}
	}

	// 2. 单字符/双字符关键词独立词匹配
	// 例如 "TD" 必须独立出现，不能是 "TD-RFID"
	// 中文关键词允许子串匹配（"TD退订"包含"TD"和"退订"）
	lower := strings.ToLower(content)
	for _, kw := range smsUnsubscribeKeywords {
		if isAsciiKeyword(kw) {
			if isStandaloneWord(lower, strings.ToLower(kw)) {
				return kw
			}
		} else {
			// 中文关键词子串匹配
			if strings.Contains(content, kw) {
				return kw
			}
		}
	}
	return ""
}

// isAsciiKeyword 判断关键词是否为纯 ASCII（英文/数字）
func isAsciiKeyword(kw string) bool {
	for _, r := range kw {
		if r > 127 {
			return false
		}
	}
	return true
}

// isStandaloneWord 判断关键词是否作为独立词出现
// 独立词定义：前后为非字母数字字符（或字符串边界）
func isStandaloneWord(content, kw string) bool {
	if kw == "" {
		return false
	}
	idx := strings.Index(content, kw)
	for idx >= 0 {
		// 检查前一个字符
		beforeOK := idx == 0
		if !beforeOK {
			r := rune(content[idx-1])
			beforeOK = !isAlnum(r)
		}
		// 检查后一个字符
		afterIdx := idx + len(kw)
		afterOK := afterIdx >= len(content)
		if !afterOK {
			r := rune(content[afterIdx])
			afterOK = !isAlnum(r)
		}
		if beforeOK && afterOK {
			return true
		}
		next := idx + 1
		if next >= len(content) {
			break
		}
		idx = strings.Index(content[next:], kw)
		if idx < 0 {
			break
		}
		idx += next
	}
	return false
}

// isAlnum 判断 rune 是否为字母或数字
func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// smsUnsubscribeServiceWithDB 内部辅助：使用指定 db 创建服务（测试用）
func smsUnsubscribeServiceWithDB(db *gorm.DB) *SmsUnsubscribeService {
	return NewSmsUnsubscribeService(repository.NewSmsUnsubscribeRepository(db))
}
