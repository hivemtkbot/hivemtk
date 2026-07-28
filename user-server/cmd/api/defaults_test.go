package main

import "testing"

// TestDefaultPortDocsConsistency 验证 user-server 兜底端口常量与文档源一致。
//
// 文档源：
//   - user-server/docs/dev/DEVELOPMENT.md §2.4 端口对照表
//     | 8204 | user-server | Gin HTTP |
//     | 8203 | Redis       |
//   - user-server/Dockerfile:57 ENV SERVER_PORT=8204
//   - user-web/bridge/src/core/constants.js DEFAULT_USER_SERVER.port=8204
//
// 调整请同步：
//   - DEVELOPMENT.md §2.4
//   - user-server/Dockerfile ENV SERVER_PORT
//   - user-web/bridge/src/core/constants.js DEFAULT_USER_SERVER.port
//   - user-web/bridge/docs/DEFAULTS.md §2.1
func TestDefaultPortDocsConsistency(t *testing.T) {
	t.Run("DefaultListenPort", func(t *testing.T) {
		if DefaultListenPort != "8204" {
			t.Errorf("DefaultListenPort 应为 8204（DEVELOPMENT.md §2.4 + Dockerfile ENV），实际 %s", DefaultListenPort)
		}
	})
	t.Run("DefaultRedisPort", func(t *testing.T) {
		if DefaultRedisPort != "8203" {
			t.Errorf("DefaultRedisPort 应为 8203（DEVELOPMENT.md §2.4），实际 %s", DefaultRedisPort)
		}
	})
	t.Run("NonSoftStartup", func(t *testing.T) {
		// 禁止空值兜底
		if DefaultListenPort == "" {
			t.Error("DefaultListenPort 不允许为空（禁软启动）")
		}
		if DefaultRedisPort == "" {
			t.Error("DefaultRedisPort 不允许为空（禁软启动）")
		}
	})
	t.Run("AlignWithBridge", func(t *testing.T) {
		// 与 bridge constants.DEFAULT_USER_SERVER.port 隐式一致：
		// bridge 端 port=8204（number），此处 DefaultListenPort="8204"（string）。
		// 此断言以字符串字面量形式验证两侧数字一致，防止文档漂移。
		if DefaultListenPort != "8204" {
			t.Error("DefaultListenPort 与 bridge DEFAULT_USER_SERVER.port(8204) 不一致")
		}
	})
}
