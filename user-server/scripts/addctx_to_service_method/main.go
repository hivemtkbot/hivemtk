// 工具:addctx_to_service_method
// 给 Service 方法(以 Service 结尾的 receiver)签名加 ctx context.Context 参数
// 同时,内部 s.xxxxRepo.X(...) 调用自动加 ctx。
//
// 思路:
//   1. 识别 Service struct(以 Service 结尾的 receiver 类型)
//   2. 给所有方法加 ctx context.Context 参数
//   3. 给方法体内的 s.xxxxRepo.X(...) / s.repo.X(...) 等调用加 ctx
//
// 用法:
//   go run scripts/addctx_to_service_method/main.go -dir=internal/email/service
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
		return 0, err
	}

	// 检查是否有 context 导入
	hasContextImport := false
	for _, imp := range f.Imports {
		if imp.Path.Value == "\"context\"" {
			hasContextImport = true
			break
		}
	}

	fixes := 0
	modified := false
	// 收集本文件中所有 Service 字段
	fieldMap := collectServiceFields(f)

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Type == nil || fn.Body == nil {
			continue
		}
		// 仅处理方法(有 receiver)
		if fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		// 仅处理 Service 类型
		recvName := receiverName(fn.Recv.List[0].Type)
		if !strings.HasSuffix(recvName, "Service") && !strings.HasSuffix(recvName, "service") {
			continue
		}
		// 仅处理非接口方法
		if isInterfaceMethod(f, fn) {
			continue
		}
		// 跳过已经有 ctx 的
		if hasContextParam(fn) {
			// 但仍修复内部调用
			subFixes := fixCallsInBody(fn, fieldMap)
			fixes += subFixes
			if subFixes > 0 {
				modified = true
			}
			continue
		}

		// 给方法签名加 ctx 参数
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
			// 插入到参数列表最前面
			newList := make([]*ast.Field, 0, len(fn.Type.Params.List)+1)
			newList = append(newList, ctxField)
			newList = append(newList, fn.Type.Params.List...)
			fn.Type.Params.List = newList
		}
		modified = true
		fixes++

		// 修复内部调用
		subFixes := fixCallsInBody(fn, fieldMap)
		fixes += subFixes
	}

	if !modified {
		return 0, nil
	}

	// 如果加了 ctx 但没 import context,加上
	if !hasContextImport {
		// 找到 imports 块,在最后追加 context
		hasContextImportAdded := false
		for _, imp := range f.Imports {
			if imp.Path.Value == "\"context\"" {
				hasContextImportAdded = true
				break
			}
		}
		if !hasContextImportAdded {
			// 创建一个新的 ImportSpec
			newImp := &ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: "\"context\"",
				},
			}
			// 找到 GenDecl 中带 IMPORT 标记的
			for _, decl := range f.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.IMPORT {
					continue
				}
				gd.Specs = append(gd.Specs, newImp)
				break
			}
		}
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

func fixCallsInBody(fn *ast.FuncDecl, fieldMap map[string]bool) int {
	fixes := 0
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// sel.X 形式 1:直接是 xxxxRepo / xxxxService 等
		if xIdent, ok := sel.X.(*ast.Ident); ok {
			if !isRepoField(xIdent.Name, fieldMap) {
				return true
			}
			methodName := sel.Sel.Name
			if !isLikelyRepoMethod(methodName) {
				return true
			}
			// 检查第一个实参是否已经是 ctx
			if len(call.Args) > 0 {
				if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "ctx" {
					return true
				}
			}
			ctxArg := &ast.Ident{Name: "ctx"}
			newArgs := make([]ast.Expr, 0, len(call.Args)+1)
			newArgs = append(newArgs, ctxArg)
			newArgs = append(newArgs, call.Args...)
			call.Args = newArgs
			fixes++
			return true
		}

		// sel.X 形式 2:嵌套的 s.ruleRepo.X
		innerSel, ok := sel.X.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := innerSel.X.(*ast.Ident); ok && (id.Name == "s" || id.Name == "r" || fieldMap[id.Name]) {
			methodName := sel.Sel.Name
			if !isLikelyRepoMethod(methodName) {
				return true
			}
			if len(call.Args) > 0 {
				if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "ctx" {
					return true
				}
			}
			ctxArg := &ast.Ident{Name: "ctx"}
			newArgs := make([]ast.Expr, 0, len(call.Args)+1)
			newArgs = append(newArgs, ctxArg)
			newArgs = append(newArgs, call.Args...)
			call.Args = newArgs
			fixes++
		}
		return true
	})
	return fixes
}

func isRepoField(name string, fieldMap map[string]bool) bool {
	if fieldMap[name] {
		return true
	}
	// 公共后缀检查
	if strings.HasSuffix(name, "Repo") || strings.HasSuffix(name, "repo") ||
		strings.HasSuffix(name, "Service") || strings.HasSuffix(name, "service") ||
		strings.HasSuffix(name, "DB") || strings.HasSuffix(name, "DAO") {
		return true
	}
	return false
}

func isLikelyRepoMethod(name string) bool {
	skipMethods := map[string]bool{
		"WithContext": true,
		"GetDB":       true,
		"SetDB":       true,
		"DB":          true,
	}
	if skipMethods[name] {
		return false
	}
	if len(name) == 0 || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	return true
}

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverName(t.X)
	}
	return ""
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

func isInterfaceMethod(file *ast.File, fn *ast.FuncDecl) bool {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
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
			for _, m := range iface.Methods.List {
				if len(m.Names) == 0 {
					continue
				}
				for _, name := range m.Names {
					if name.Name == fn.Name.Name {
						return true
					}
				}
			}
		}
	}
	return false
}

func collectServiceFields(f *ast.File) map[string]bool {
	fieldMap := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, f := range st.Fields.List {
			for _, name := range f.Names {
				if name == nil {
					continue
				}
				n := name.Name
				if strings.HasSuffix(n, "Repo") || strings.HasSuffix(n, "repo") ||
					strings.HasSuffix(n, "Service") || strings.HasSuffix(n, "service") ||
					strings.HasSuffix(n, "DB") || strings.HasSuffix(n, "DAO") {
					fieldMap[n] = true
				}
			}
		}
		return true
	})
	return fieldMap
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
