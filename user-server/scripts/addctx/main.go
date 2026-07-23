// 工具:addctx
// 批量给方法签名加 ctx context.Context。
// 函数体内 r.db 引用由 perl 脚本改为链式 r.db.WithContext(ctx)。
//
// 用法:
//   go run scripts/addctx/main.go -dir=internal/repository
package main

import (
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

func main() {
	dir := flag.String("dir", "", "目标目录,必填")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "用法: addctx -dir=internal/repository")
		os.Exit(1)
	}

	count := 0
	err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		n, e := processFile(path)
		if e == nil {
			count += n
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "WARN", err)
	}
	fmt.Printf("共加 ctx 参数 %d 处\n", count)
}

func processFile(path string) (int, error) {
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

	count := 0

	// 先处理 interface 类型的方法
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			t, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			iface, ok := t.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			if iface.Methods == nil {
				continue
			}
			for _, m := range iface.Methods.List {
				ft, ok := m.Type.(*ast.FuncType)
				if !ok || ft.Params == nil {
					continue
				}
				if len(m.Names) == 0 {
					continue
				}
				// 检查是否已有 context.Context
				hasCtx := false
				for _, p := range ft.Params.List {
					if exprToString(p.Type) == "context.Context" {
						hasCtx = true
						break
					}
				}
				if hasCtx {
					continue
				}
				ctxField := &ast.Field{
					Names: []*ast.Ident{{Name: "ctx"}},
					Type: &ast.SelectorExpr{
						X:   &ast.Ident{Name: "context"},
						Sel: &ast.Ident{Name: "Context"},
					},
				}
				ft.Params.List = append([]*ast.Field{ctxField}, ft.Params.List...)
				count++
			}
		}
	}

	// 处理方法实现
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}

		// 检查是否已有 context.Context
		hasCtx := false
		if fn.Type.Params != nil {
			for _, p := range fn.Type.Params.List {
				if exprToString(p.Type) == "context.Context" {
					hasCtx = true
					break
				}
			}
		}
		if hasCtx {
			continue
		}

		// 插入 ctx 参数
		ctxField := &ast.Field{
			Names: []*ast.Ident{{Name: "ctx"}},
			Type: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "context"},
				Sel: &ast.Ident{Name: "Context"},
			},
		}
		if fn.Type.Params == nil {
			fn.Type.Params = &ast.FieldList{List: []*ast.Field{ctxField}}
		} else {
			fn.Type.Params.List = append([]*ast.Field{ctxField}, fn.Type.Params.List...)
		}
		count++
	}

	if count == 0 {
		return 0, nil
	}

	// 输出 AST
	var sb strings.Builder
	if err := printer.Fprint(&sb, fset, f); err != nil {
		return 0, err
	}
	newCode := sb.String()

	// 确保 import context
	if !strings.Contains(newCode, "\"context\"") {
		newCode = addContextImport(newCode)
	}

	if newCode == original {
		return 0, nil
	}

	if err := os.WriteFile(path, []byte(newCode), 0644); err != nil {
		return 0, err
	}
	return count, nil
}

func addContextImport(code string) string {
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import (") {
			for j := i + 1; j < len(lines); j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), ")") {
					insertLine := "\t\"context\""
					newLines := make([]string, 0, len(lines)+1)
					newLines = append(newLines, lines[:j]...)
					newLines = append(newLines, insertLine)
					newLines = append(newLines, lines[j:]...)
					return strings.Join(newLines, "\n")
				}
			}
		}
	}
	return code
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
