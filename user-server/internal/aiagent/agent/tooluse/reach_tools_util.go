package tooluse

import (
	"fmt"
)

// getArgStringMap 安全获取 map[string]string 参数
// 支持 map[string]interface{}（JSON 反序列化结果）
// getArgStringMap 从 args 取 map[string]string（向旧路径兼容：非 string 值会被 fmt.Sprintf 转字符串）
//
// 保留此函数供 SMS 等需要 map[string]string 的场景
// 新代码请优先使用 getArgMap（保留原始类型）
func getArgStringMap(args map[string]any, key string) map[string]string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		out := make(map[string]string, len(m))
		for k, val := range m {
			if s, ok := val.(string); ok {
				out[k] = s
			} else {
				out[k] = fmt.Sprintf("%v", val)
			}
		}
		return out
	}

	if m, ok := v.(map[string]string); ok {
		return m
	}
	return nil
}

// getArgInterfaceSlice 安全获取 []interface{} 参数
func getArgInterfaceSlice(args map[string]any, key string) []any {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}

// getArgMap 安全获取 map[string]interface{} 参数（保留原始类型，不像 getArgStringMap 那样把数字/布尔强转字符串）
//
// 用于 BatchSendItem.Payload / EnqueueJobRequest.Payload 等需要保留类型的场景
func getArgMap(args map[string]any, key string) map[string]any {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// parseUint 字符串转 uint（解析失败返回 0）
func parseUint(s string) uint {
	if s == "" {
		return 0
	}
	var n uint
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0
	}
	return n
}

