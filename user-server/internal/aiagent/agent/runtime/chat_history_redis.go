package agent_runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"hivemtk-user/internal/aiagent/agent/portcontract"
	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)


// ChatHistoryRedisAdapter 基于 Redis 的会话历史适配器
//
// 实现 portcontract.ChatHistoryPort 接口
//
// 装配：service 包的 tool_ports_adapter.go 通过 setter 注入到 AssetBundleLoader
type ChatHistoryRedisAdapter struct {
	defaultLimit int
	ttl          time.Duration
}

// NewChatHistoryRedisAdapter 创建 Redis 会话历史适配器
//
// defaultLimit: FetchHistory 的 limit=0 时使用的默认值（建议 10）
func NewChatHistoryRedisAdapter(defaultLimit int) *ChatHistoryRedisAdapter {
	if defaultLimit <= 0 {
		defaultLimit = 10
	}
	return &ChatHistoryRedisAdapter{
		defaultLimit: defaultLimit,
		ttl:          7 * 24 * time.Hour,
	}
}

// chatHistoryKey 构造 Redis key
func chatHistoryKey(sessionID string) string {
	return "chat_history:" + sessionID
}

// FetchHistory 拉取指定会话最近 N 条历史消息
//
// 实现 portcontract.ChatHistoryPort
//
// 行为：
//  1. 从 Redis List 末尾取 limit 条（LRANGE -limit -1）
//  2. 反序列化每个 List 元素为 model.AssetBundleMessage
//  3. 按时间正序返回（Redis List 本身就是写入顺序）
//  4. Redis 不可用时返回空切片（不报错，避免阻塞 Weave）
func (a *ChatHistoryRedisAdapter) FetchHistory(ctx context.Context, sessionID string, limit int) ([]model.AssetBundleMessage, error) {
	if sessionID == "" {
		return nil, errors.New("session_id required")
	}
	if limit <= 0 {
		limit = a.defaultLimit
	}
	c := cache.GetGlobalCache()
	key := chatHistoryKey(sessionID)
	start := int64(-limit)
	stop := int64(-1)
	rawList, err := c.LRange(ctx, key, start, stop)
	if err != nil {
		logger.Warnf("[chat_history] fetch failed session=%s err=%v", sessionID, err)
		return []model.AssetBundleMessage{}, nil
	}
	if len(rawList) == 0 {
		return []model.AssetBundleMessage{}, nil
	}
	out := make([]model.AssetBundleMessage, 0, len(rawList))
	for _, raw := range rawList {
		var msg model.AssetBundleMessage
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			logger.Warnf("[chat_history] unmarshal failed session=%s err=%v", sessionID, err)
			continue
		}
		out = append(out, msg)
	}
	logger.Debugf("[chat_history] fetch ok session=%s limit=%d got=%d", sessionID, limit, len(out))
	return out, nil
}

// AppendHistory 追加一条历史消息
//
// 实现 portcontract.ChatHistoryPort
//
// 行为：
//  1. JSON 序列化消息
//  2. RPush 到 Redis List（按时间正序追加）
//  3. 续期 TTL（活跃会话永不过期，闲置 7 天后清理）
//  4. Redis 不可用时只 warn，不报错（保证主流程不阻塞）
func (a *ChatHistoryRedisAdapter) AppendHistory(ctx context.Context, sessionID string, msg model.AssetBundleMessage) error {
	if sessionID == "" {
		return errors.New("session_id required")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal msg: %w", err)
	}
	c := cache.GetGlobalCache()
	key := chatHistoryKey(sessionID)
	if err := c.RPush(ctx, key, string(data), a.ttl); err != nil {
		logger.Warnf("[chat_history] append failed session=%s err=%v", sessionID, err)
		return nil
	}
	return nil
}

// 编译期断言：ChatHistoryRedisAdapter 实现 portcontract.ChatHistoryPort
var _ portcontract.ChatHistoryPort = (*ChatHistoryRedisAdapter)(nil)

