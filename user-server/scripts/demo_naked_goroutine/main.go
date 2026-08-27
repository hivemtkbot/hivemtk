package main

// _demo_naked_goroutine.go —— check_naked_goroutine.sh 自检 demo
//
// 这个文件仅用于验证 scripts/check_naked_goroutine.sh 能正确识别裸 go func( 调用。
// 不要在生产代码中保留此模式——裸 goroutine 没有 panic recover，会击穿 gin.Recovery。
//
// 手动验证：
//   bash scripts/check_naked_goroutine.sh | grep demo_naked_goroutine
// 应只命中本文件 line 21（真正的裸 goroutine），而注释中的 `// go func(` 不应被命中。

import (
	"fmt"
)

func main() {
	// ❌ 错误示例：裸 goroutine（CI 应报错）
	// 未来此处应改为 utils.SafeGo / utils.SafeGoDetached
	go func() {
		fmt.Println("naked goroutine - 进程会在 panic 时崩溃")
	}()

	// ✅ 正确示例（注释形式，不被脚本识别为裸调用）：
	// go func() {
	//     fmt.Println("commented naked goroutine - 脚本应豁免注释")
	// }()
}