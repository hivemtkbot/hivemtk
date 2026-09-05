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
