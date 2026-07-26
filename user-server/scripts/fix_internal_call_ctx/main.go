// 工具:fix_internal_call_ctx
// 智能修复 service 内部方法调用 (s.X / r.X / c.X)：
//  1. 如果目标方法定义有 ctx 但调用处没传，则补 ctx
//  2. 如果目标方法定义无 ctx 但调用处传了 ctx，则移除 ctx
//
// 仅修改本文件内定义的 method（基于 receiver 类型 / 同名 method / 同包），
// 跨文件的 method 暂不处理（由 controller → service 调用触发）。
//
// 用法:
//
//	go run scripts/fix_internal_call_ctx/main.go internal/service
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

type methodInfo struct {
	hasCtx bool
}

func main() {
	dir := flag.String("dir", "internal/service", "目标目录")
	flag.Parse()

	totalFixes := 0
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
			fmt.Printf("  ✓ %s 修复 %d 处\n", path, n)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "WALK ERR", err)
	}
	fmt.Printf("共修复 %d 处\n", totalFixes)
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
		return 0, nil // 解析失败的先跳过
	}

	// 收集本文件中定义的方法：methodName -> hasCtx
	methods := collectMethods(f)

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

		// 只处理 s.X / r.X / c.X 等单 ident receiver
		recv, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if recv.Name == "ctx" || recv.Name == "context" {
			return true
		}

		methodName := sel.Sel.Name
		info, found := methods[methodName]
		if !found {
			return true
		}

		hasCtxArg := false
		if len(call.Args) > 0 {
			if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "ctx" {
				hasCtxArg = true
			}
		}

		if info.hasCtx && !hasCtxArg {
			// 需要补 ctx
			ctxIdent := &ast.Ident{Name: "ctx"}
			newArgs := make([]ast.Expr, 0, len(call.Args)+1)
			newArgs = append(newArgs, ctxIdent)
			newArgs = append(newArgs, call.Args...)
			call.Args = newArgs
			fixes++
		} else if !info.hasCtx && hasCtxArg {
			// 需要移除 ctx
			call.Args = call.Args[1:]
			fixes++
		}
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

func collectMethods(f *ast.File) map[string]methodInfo {
	methods := make(map[string]methodInfo)
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		info := methodInfo{hasCtx: hasContextParam(fn)}
		methods[fn.Name.Name] = info
	}
	// interface methods
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			iface, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			if iface.Methods == nil {
				continue
			}
			for _, m := range iface.Methods.List {
				if len(m.Names) == 0 {
					continue
				}
				ft, ok := m.Type.(*ast.FuncType)
				if !ok {
					continue
				}
				methods[m.Names[0].Name] = methodInfo{hasCtx: firstParamIsContext(ft)}
			}
		}
	}
	return methods
}

func hasContextParam(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil {
		return false
	}
	return firstParamIsContext(fn.Type)
}

func firstParamIsContext(ft *ast.FuncType) bool {
	if ft.Params == nil || len(ft.Params.List) == 0 {
		return false
	}
	return exprToString(ft.Params.List[0].Type) == "context.Context"
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
