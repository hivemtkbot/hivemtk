package model

// IntentExample 意图示例句向量表（M4 I-1 Embedding 中间层持久化锚点）
//
// X-7 约束：与 KB chunk 表（knowledge_chunks）分表，避免污染 RAG 检索语料。
// Vector 存 pgvector 字面量 '[v1,v2,...]'，维度硬性 1024（bge-m3 对齐）。
type IntentExample struct {
	ID     uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Intent string `gorm:"type:varchar(64);not null;index" json:"intent"`
	Text   string `gorm:"type:varchar(512);not null;uniqueIndex" json:"text"`
	Vector string `gorm:"type:vector(1024)" json:"-"`
}

// TableName GORM 表名
func (IntentExample) TableName() string { return "intent_examples" }
