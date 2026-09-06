package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// 宏动作类型
const (
	MacroActAddTag      = "add_tag"
	MacroActAddNote     = "add_note"
	MacroActAssign      = "assign"
	MacroActClose       = "close"
	MacroActSendMessage = "send_message"
	MacroActSetPriority = "set_priority"
)

// MacroAction 单动作
type MacroAction struct {
	Type  string `json:"type" binding:"required"`
	Value string `json:"value"`
}

// MacroService 宏服务
type MacroService struct {
	db     *gorm.DB
	csPlus *CustomerServicePlusService
}

// NewMacroService 构造
func NewMacroService(gdb *gorm.DB) *MacroService {
	return &MacroService{db: gdb, csPlus: NewCustomerServicePlusServiceFromGlobal()}
}

// NewMacroServiceFromGlobal 便捷构造
func NewMacroServiceFromGlobal() *MacroService { return NewMacroService(db.GetDB()) }

// Create 创建宏
func (s *MacroService) Create(ctx context.Context, name string, actions []MacroAction) (*model.Macro, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("宏名称不能为空")
	}
	if len(actions) == 0 {
		return nil, fmt.Errorf("至少一个动作")
	}
	for _, a := range actions {
		switch a.Type {
		case MacroActAddTag, MacroActAddNote, MacroActAssign, MacroActClose, MacroActSendMessage, MacroActSetPriority:
		default:
			return nil, fmt.Errorf("不支持的动作类型: %s", a.Type)
		}
	}
	raw, err := json.Marshal(actions)
	if err != nil {
		return nil, err
	}
	m := &model.Macro{Name: name, Actions: string(raw)}
	if err := s.db.WithContext(ctx).Create(m).Error; err != nil {
		return nil, err
	}
	return m, nil
}

// List 宏列表
func (s *MacroService) List(ctx context.Context) ([]*model.Macro, error) {
	var list []*model.Macro
	err := s.db.WithContext(ctx).Order("id ASC").Find(&list).Error
	return list, err
}

// Delete 删除宏
func (s *MacroService) Delete(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&model.Macro{}, id).Error
}

// ApplyResult 应用结果
type ApplyResult struct {
	Executed []string `json:"executed"`
	Failed   []string `json:"failed"`
}

// Apply 对会话执行宏动作序列
func (s *MacroService) Apply(ctx context.Context, macroID uint, sessionID, operator string) (*ApplyResult, error) {
	var m model.Macro
	if err := s.db.WithContext(ctx).First(&m, macroID).Error; err != nil {
		return nil, err
	}
	var actions []MacroAction
	if err := json.Unmarshal([]byte(m.Actions), &actions); err != nil {
		return nil, fmt.Errorf("宏动作解析失败: %w", err)
	}
	res := &ApplyResult{}
	for _, a := range actions {
		var err error
		switch a.Type {
		case MacroActAddTag:

			err = s.appendSessionTag(ctx, sessionID, a.Value)
		case MacroActAddNote:
			_, err = s.csPlus.AddInternalNote(ctx, sessionID, fmt.Sprintf("[宏:%s] %s", m.Name, a.Value), "0", operator)
		case MacroActAssign:
			err = s.assignSession(ctx, sessionID, a.Value)
		case MacroActClose:
			err = s.closeSession(ctx, sessionID)
		case MacroActSendMessage:
			err = s.enqueueOutbound(ctx, sessionID, a.Value)
		case MacroActSetPriority:
			lvl := 0
			fmt.Sscanf(a.Value, "%d", &lvl)
			err = s.csPlus.SetSessionPriority(ctx, sessionID, lvl)
		}
		if err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", a.Type, err))
		} else {
			res.Executed = append(res.Executed, a.Type)
		}
	}
	return res, nil
}

func (s *MacroService) appendSessionTag(ctx context.Context, sessionID, tagCode string) error {
	if strings.TrimSpace(tagCode) == "" {
		return fmt.Errorf("标签为空")
	}
	g := s.db
	var tagsRaw string
	if err := g.WithContext(ctx).Table("customer_sessions").
		Select("COALESCE(tags,'')").Where("session_id = ?", sessionID).Scan(&tagsRaw).Error; err != nil {
		return err
	}
	var arr []string
	if json.Unmarshal([]byte(tagsRaw), &arr) != nil {
		arr = []string{}
	}
	for _, t := range arr {
		if t == tagCode {
			return nil
		}
	}
	arr = append(arr, tagCode)
	merged, _ := json.Marshal(arr)
	return g.WithContext(ctx).Table("customer_sessions").
		Where("session_id = ?", sessionID).
		Update("tags", string(merged)).Error
}

func (s *MacroService) assignSession(ctx context.Context, sessionID, agentID string) error {
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("坐席为空")
	}
	return s.db.WithContext(ctx).Table("customer_sessions").
		Where("session_id = ?", sessionID).
		Update("agent_id", agentID).Error
}

func (s *MacroService) closeSession(ctx context.Context, sessionID string) error {
	return s.db.WithContext(ctx).Table("customer_sessions").
		Where("session_id = ?", sessionID).
		Update("status", "closed").Error
}

func (s *MacroService) enqueueOutbound(ctx context.Context, sessionID, content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("消息内容为空")
	}
	g := s.db

	var sess struct {
		Platform string
		Account  string
	}
	if err := g.WithContext(ctx).Table("customer_sessions").
		Select("platform, COALESCE(account_id,'') AS account").
		Where("session_id = ?", sessionID).Scan(&sess).Error; err != nil {
		return err
	}

	now := time.Now()
	rec := &model.MessageHub{
		Platform:       sess.Platform,
		MsgID:          fmt.Sprintf("macro_%s_%d", sessionID, now.UnixNano()),
		AccountID:      sess.Account,
		Direction:      "outbound",
		Status:         "pending",
		MsgType:        "text",
		SenderID:       "system",
		SenderName:     "宏消息",
		Content:        content,
		ConversationID: sessionID,
		TraceID:        "macro",
		SentAt:         now,
	}
	return g.WithContext(ctx).Create(rec).Error
}
