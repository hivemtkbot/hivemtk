package common

import (
	"encoding/json"
	"io"

	"marketing/internal/pkg/utils/httpclient"
)

func GetRequest(url string) (map[string]any, error) {
	resp, err := httpclient.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 转换为字典
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return result, err
	}

	// 使用结果
	return result, nil
}
