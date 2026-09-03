package middleware

import "testing"

// init R55 T9：测试环境启用 testModeGate（生产代码零 testing 依赖）
func init() {
	testModeGate = func() bool { return testing.Testing() }
}
