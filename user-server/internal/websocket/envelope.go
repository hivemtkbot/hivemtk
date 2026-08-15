package websocket

import (
	"encoding/json"
	"time"
)


// NewEnvelope 构造一个带 seq + ts 的 Envelope
//
// seq=0 表示不分配序号（用于 heartbeat 等控制帧）
func NewEnvelope(seq uint64, messageType string, payload any) (*Envelope, error) {
	env := &Envelope{
		Seq:  seq,
		TS:   time.Now().UnixMilli(),
		Type: messageType,
	}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		env.Payload = raw
	}
	return env, nil
}

// MustEnvelope 同 NewEnvelope，序列化失败返回仅含 type 的最小 envelope
func MustEnvelope(seq uint64, messageType string, payload any) *Envelope {
	env, err := NewEnvelope(seq, messageType, payload)
	if err != nil {
		return &Envelope{Seq: seq, TS: time.Now().UnixMilli(), Type: messageType}
	}
	return env
}

// MarshalBytes 把 Envelope 序列化为 wire 字节
func (e *Envelope) MarshalBytes() ([]byte, error) {
	return json.Marshal(e)
}

// sendEnvelope 内部统一：分配 seq（可选）+ 序列化 + 投递到 client.send channel
//
// 行为：
//   - assignSeq=true 时调用 NextSeq() 分配新序号
//   - 投递失败（channel 满）返回 false，调用方应记录 dropped
//
// 注意：ack 跟踪由调用方决定，SendEnvelope 不自动 track，避免污染 heartbeat 等
func sendEnvelope(client *Client, env *Envelope) bool {
	if client == nil || env == nil {
		return false
	}
	bytes, err := env.MarshalBytes()
	if err != nil {
		return false
	}
	select {
	case client.send <- bytes:
		return true
	case <-time.After(500 * time.Millisecond):
		return false
	default:
		return false
	}
}

