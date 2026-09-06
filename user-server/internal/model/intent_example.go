package model

type IntentExample struct {
	ID     uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Intent string `gorm:"type:varchar(64);not null;index" json:"intent"`
	Text   string `gorm:"type:varchar(512);not null;uniqueIndex" json:"text"`
	Vector string `gorm:"type:vector(1024)" json:"-"`
}

// TableName GORM 表名
func (IntentExample) TableName() string { return "intent_examples" }
