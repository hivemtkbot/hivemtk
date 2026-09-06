package tooluse

import (
	"fmt"
)

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
