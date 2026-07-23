package service

import (
	"encoding/json"

	"marketing/internal/model"
)

// GetCustomerEventData 解析 CustomerEvent.EventData (JSON string) 为 map。
//
// 原先在 model.CustomerEvent 上以方法形式提供，违反五层架构(§3.6 Model 禁止业务方法)。
// 这里把它下移到 service 层作为 free function，model 仅保留 GORM Hook 与 TableName。
func GetCustomerEventData(e *model.CustomerEvent) map[string]any {
	if e == nil || e.EventData == "" {
		return map[string]any{}
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(e.EventData), &data); err != nil {
		return map[string]any{}
	}
	return data
}

// SetCustomerEventData 将 map 序列化为 JSON string 写入 CustomerEvent.EventData。
// 替代原先 model.CustomerEvent.SetEventData 方法。
func SetCustomerEventData(e *model.CustomerEvent, data map[string]any) error {
	if e == nil {
		return nil
	}
	if data == nil {
		data = map[string]any{}
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	e.EventData = string(jsonData)
	return nil
}
