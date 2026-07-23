// 工具:fix_sdb_withctx
// 修复 Service 中假 ctx 迁移:
//   - 函数签名已带 ctx context.Context
//   - 函数体中的 s.db / s.dbOrDefault / s.gormDB / s.gormDBOrDefault / s.client
//     未使用 ctx 链式 WithContext(ctx)
//
// 思路:
//   1. 用 Go AST 解析,识别有 ctx context.Context 参数的方法
//   2. 在方法体内,把所有 s.db. / s.dbOrDefault. / s.gormDB. / s.gormDBOrDefault. / s.client.
//      改为 s.db.WithContext(ctx). / ...
//   3. 跳过赋值语句的右侧出现的情况(如 q := s.db.Model(...)),一并处理
//   4. 同时处理 s.db.Where("id = ?", id).Delete 这种链式
//
// 用法:
//   go run scripts/fix_sdb_withctx/main.go -dir=internal/service
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

var dbVarNames = []string{
	"s.db",
	"s.dbOrDefault",
	"s.gormDB",
	"s.gormDBOrDefault",
	"s.client",
}

func main() {
	dir := flag.String("dir", "internal/service", "目标目录")
	flag.Parse()

	totalFixes := 0
	totalFiles := 0
	err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		n, e := fixFile(path)
		if e == nil && n > 0 {
			totalFixes += n
			totalFiles++
			fmt.Printf("  ✓ %s 修复 %d 处\n", path, n)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "WALK ERR", err)
	}
	fmt.Printf("共修复 %d 处,涉及 %d 个文件\n", totalFixes, totalFiles)
}

func fixFile(path string) (int, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	original := string(src)

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, original, parser.ParseComments)
	if err != nil {
		return 0, err
	}

	fixes := 0
	// 1. 收集每个 FuncDecl 的 ctx 状态
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Type == nil || fn.Type.Params == nil {
			continue
		}
		hasCtx := hasContextParam(fn)
		if !hasCtx {
			continue
		}

		// 2. 在函数体中找 db 调用并插入 WithContext(ctx)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// 检查 call.Fun 是 s.db / s.db.X / s.dbOrDefault / s.dbOrDefault.X 等
			x := sel.X
			var receiverName string
			switch v := x.(type) {
			case *ast.SelectorExpr:
				if id, ok := v.X.(*ast.Ident); ok && id.Name == "s" {
					receiverName = v.Sel.Name
				}
			case *ast.Ident:
				if v.Name == "s" {
					// s.db 形式
					receiverName = ""
				}
			}
			if receiverName != "db" && receiverName != "dbOrDefault" &&
				receiverName != "gormDB" && receiverName != "gormDBOrDefault" &&
				receiverName != "client" {
				// 也可能是 s.db 这种没有 receiver 的根
				if _, ok := x.(*ast.Ident); ok {
					// 检查 x 是否为 s
					if id, ok := x.(*ast.Ident); ok && id.Name != "s" {
						return true
					}
				}
				return true
			}

			// 找到 s.db / s.dbOrDefault / ... 这种调用
			// sel.Sel.Name 是方法名(如 First/Create/Where...)
			// 跳过的方法:
			if sel.Sel.Name == "WithContext" {
				return true
			}
			// 如果方法名是 "DB" 等就跳过
			if sel.Sel.Name == "DB" || sel.Sel.Name == "Raw" {
				// 允许,但是注意 Raw 不需要 WithContext
			}

			// 修改:把 s.db 替换为 s.db.WithContext(ctx)
			// 也就是在 sel.X 上套一层 call
			// 原: sel.X.Sel.Method(...)
			// 新: sel.X.Sel.WithContext(ctx).Method(...)
			withCtxCall := &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   sel.X,
					Sel: &ast.Ident{Name: "WithContext"},
				},
				Args: []ast.Expr{&ast.Ident{Name: "ctx"}},
			}
			sel.X = withCtxCall
			fixes++
			return true
		})
	}

	if fixes == 0 {
		return 0, nil
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, f); err != nil {
		return 0, err
	}
	newCode := buf.String()
	if newCode == original {
		return 0, nil
	}
	if err := os.WriteFile(path, []byte(newCode), 0644); err != nil {
		return 0, err
	}
	return fixes, nil
}

func hasContextParam(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil {
		return false
	}
	for _, p := range fn.Type.Params.List {
		if exprToString(p.Type) == "context.Context" {
			return true
		}
	}
	return false
}

func exprToString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprToString(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(v.X)
	default:
		return ""
	}
}
