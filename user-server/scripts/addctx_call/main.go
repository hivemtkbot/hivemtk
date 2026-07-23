// 工具:addctx_call
// 扫描 Repository 目录,找到所有 r.MethodName(...) 形式的方法调用,
// 如果 MethodName 是当前包内已加 ctx 的方法,自动在第一个实参前插入 ctx。
//
// 用法:
//   go run scripts/addctx_call/main.go -dir=internal/repository
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
	"sort"
	"strings"
)

func main() {
	dir := flag.String("dir", "", "目标目录,必填")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "用法: addctx_call -dir=internal/repository")
		os.Exit(1)
	}

	// 第一步:扫描所有文件,收集本包内已加 ctx 的方法名
	ctxMethods := map[string]bool{}
	if err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Type == nil || fn.Type.Params == nil {
				return true
			}
			for _, p := range fn.Type.Params.List {
				if exprToString(p.Type) == "context.Context" {
					if fn.Recv != nil && len(fn.Recv.List) > 0 && len(fn.Recv.List[0].Names) > 0 {
						// 任意 receiver 的方法都收集(只在本包内用)
						ctxMethods[fn.Name.Name] = true
					}
					break
				}
			}
			return true
		})
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "WARN", err)
	}
	fmt.Printf("本包已加 ctx 的方法数: %d\n", len(ctxMethods))

	// 第二步:处理所有文件中的方法调用
	totalFixes := 0
	if err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		n, e := fixFile(path, ctxMethods)
		if e == nil {
			totalFixes += n
		}
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "WARN", err)
	}
	fmt.Printf("共修复 %d 处方法调用\n", totalFixes)
}

func fixFile(path string, ctxMethods map[string]bool) (int, error) {
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
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok || x.Name != "r" {
			return true
		}
		methodName := sel.Sel.Name
		if !ctxMethods[methodName] {
			return true
		}
		// 检查第一个实参是否已经是 ctx
		if len(call.Args) > 0 {
			if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "ctx" {
				return true
			}
		}
		// 插入 ctx 作为第一个实参
		ctxArg := &ast.Ident{Name: "ctx"}
		newArgs := make([]ast.Expr, 0, len(call.Args)+1)
		newArgs = append(newArgs, ctxArg)
		newArgs = append(newArgs, call.Args...)
		call.Args = newArgs
		fixes++
		return true
	})

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

// 阻止 unused import 警告
var _ = sort.Strings
