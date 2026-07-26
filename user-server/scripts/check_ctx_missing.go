// check_ctx_missing.go - 静态扫描 repository 包中缺 context.Context 的方法
//
//   - 扫描 internal/repository/*.go（排除 _test.go）
//   - 识别所有以 (r *xxx) 开头的方法签名
//   - 统计缺失 ctx 参数的方法数
//   - 按文件输出缺 ctx 方法列表
//
// 用法：go run scripts/check_ctx_missing.go [包路径]
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 排除的方法（如 GetDB / WithTx 等不需要 ctx 的基础方法）
var excludedMethods = map[string]bool{
	"GetDB": true,
	"WithTx": true,
	"SetDB": true,
	"DB":    true,
}

func main() {
	repoDir := "internal/repository"
	if len(os.Args) > 1 {
		repoDir = os.Args[1]
	}

	fset := token.NewFileSet()
	totalMissing := 0
	type FileStat struct {
		Path    string
		Missing int
		Methods []string
	}
	var stats []FileStat

	entries, err := os.ReadDir(repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read dir failed: %v\n", err)
		os.Exit(1)
	}

	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(repoDir, e.Name())
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s failed: %v\n", path, err)
			continue
		}

		var missing []string
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				return true
			}
			// 只关心方法
			if fn.Recv.NumFields() == 0 {
				return true
			}
			// 接收者类型
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				_ = star
			} else {
				return true
			}

			methodName := fn.Name.Name
			if excludedMethods[methodName] {
				return true
			}

			// 检查是否包含 ctx context.Context 参数
			hasCtx := false
			if fn.Type.Params != nil {
				for _, p := range fn.Type.Params.List {
					if isContextType(p.Type) {
						hasCtx = true
						break
					}
				}
			}

			if !hasCtx {
				missing = append(missing, methodName)
			}
			return true
		})

		if len(missing) > 0 {
			stats = append(stats, FileStat{Path: path, Missing: len(missing), Methods: missing})
			totalMissing += len(missing)
		}
	}

	// 排序
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Missing > stats[j].Missing
	})

	fmt.Printf("==== Repository ctx 缺失检测 ====\n")
	fmt.Printf("扫描目录：%s\n", repoDir)
	fmt.Printf("缺 ctx 的方法总数：%d\n\n", totalMissing)

	if totalMissing == 0 {
		return
	}

	fmt.Printf("==== Top 20 缺 ctx 最多的文件 ====\n")
	for i, s := range stats {
		if i >= 20 {
			break
		}
		fmt.Printf("❌ %s: %d 个\n", s.Path, s.Missing)
		// 输出方法名
		limit := len(s.Methods)
		if limit > 10 {
			limit = 10
		}
		for j := 0; j < limit; j++ {
			fmt.Printf("   - %s\n", s.Methods[j])
		}
		if len(s.Methods) > 10 {
			fmt.Printf("   ... 还有 %d 个\n", len(s.Methods)-10)
		}
	}
}

// isContextType 检查 expr 是否为 context.Context 类型
func isContextType(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		// context.Context
		if x, ok := e.X.(*ast.Ident); ok && x.Name == "context" && e.Sel.Name == "Context" {
			return true
		}
	case *ast.Ident:
		// 已重命名为 ctx 别名
		if e.Name == "Context" {
			return true
		}
	}
	return false
}
