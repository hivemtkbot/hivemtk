// scripts/check_fake_ctx.go
// ============================================================================
// 假 ctx 迁移检测(Go AST 版)
// ----------------------------------------------------------------------------
// 检测 Service / Repository 中"签名有 ctx context.Context 但函数体未使用"的反模式
// 这类方法签名像 ctx-aware,实际行为是 nil/bg ctx,无法享受取消/超时/追踪
//
// 用法:
//   go run scripts/check_fake_ctx.go <DIR>...
// 示例:
//   go run scripts/check_fake_ctx.go internal/service internal/repository
// ============================================================================

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

type fakeMethod struct {
	File   string
	Line   int
	Struct string
	Method string
	Sig    string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: go run scripts/check_fake_ctx.go <DIR>...")
		fmt.Fprintln(os.Stderr, "示例: go run scripts/check_fake_ctx.go internal/service internal/repository")
		os.Exit(1)
	}

	dirs := os.Args[1:]
	fset := token.NewFileSet()
	var fakes []fakeMethod

	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // 跳过无权限目录
			}
			if info.IsDir() {
				// 跳过测试目录与非 Go 目录
				name := info.Name()
				if name == ".git" || name == "node_modules" || name == "vendor" || name == "bin" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			// 跳过测试文件
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			return checkFile(fset, path, &fakes)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN 遍历 %s 失败: %v\n", dir, err)
		}
	}

	// 按文件分组输出
	sort.Slice(fakes, func(i, j int) bool {
		if fakes[i].File != fakes[j].File {
			return fakes[i].File < fakes[j].File
		}
		return fakes[i].Line < fakes[j].Line
	})

	// 按文件聚合统计
	byFile := map[string]int{}
	for _, f := range fakes {
		byFile[f.File]++
	}

	fmt.Println("==== 假 ctx 迁移检测(Go AST 版)====")
	fmt.Printf("扫描目录:%v\n", dirs)
	fmt.Printf("假迁移方法总数:%d\n", len(fakes))
	if len(fakes) == 0 {
		fmt.Println("✅ 无假迁移方法")
		return
	}

	fmt.Println()
	fmt.Println("==== 按文件分布(Top 20)====")
	type fileCount struct {
		File  string
		Count int
	}
	var fcs []fileCount
	for f, c := range byFile {
		fcs = append(fcs, fileCount{f, c})
	}
	sort.Slice(fcs, func(i, j int) bool { return fcs[i].Count > fcs[j].Count })
	limit := 20
	if len(fcs) < limit {
		limit = len(fcs)
	}
	for _, fc := range fcs[:limit] {
		fmt.Printf("❌ %s: %d 个\n", fc.File, fc.Count)
	}

	fmt.Println()
	fmt.Println("==== 详细列表 ====")
	for _, f := range fakes {
		fmt.Printf("❌ %s:%d  %s.%s\n", f.File, f.Line, f.Struct, f.Method)
	}

	os.Exit(1)
}

// checkFile 解析单个文件,查找假迁移方法
func checkFile(fset *token.FileSet, path string, fakes *[]fakeMethod) error {
	src, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil // 跳过解析失败的文件
	}

	for _, decl := range src.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// 仅检测方法(有 receiver)
		if fn.Recv == nil || fn.Recv.List == nil || len(fn.Recv.List) == 0 {
			continue
		}
		// 提取 receiver 类型名
		recvType := receiverName(fn.Recv.List[0].Type)
		if recvType == "" {
			continue
		}
		// 跳过接口定义(只检测实现)
		if isInterfaceMethod(src, fn) {
			continue
		}
		// 必须有 ctx context.Context 形参
		if !hasCtxParam(fn) {
			continue
		}
		// 检查函数体是否真正使用了 ctx
		if !usesCtx(fn) {
			sig := formatSig(fn)
			line := fset.Position(fn.Pos()).Line
			*fakes = append(*fakes, fakeMethod{
				File:   path,
				Line:   line,
				Struct: recvType,
				Method: fn.Name.Name,
				Sig:    sig,
			})
		}
	}
	return nil
}

// receiverName 提取 receiver 类型名(如 *tiktokCardService → tiktokCardService)
func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverName(t.X)
	}
	return ""
}

// hasCtxParam 检查函数是否有 ctx context.Context 形参
func hasCtxParam(fn *ast.FuncDecl) bool {
	if fn.Type == nil || fn.Type.Params == nil {
		return false
	}
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if name.Name != "ctx" {
				continue
			}
			// 验证类型是否为 context.Context
			if sel, ok := field.Type.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "context" && sel.Sel.Name == "Context" {
					return true
				}
			}
		}
	}
	return false
}

// usesCtx 在函数体中查找 ctx 标识符的真实使用
// 真正使用 = 出现在调用参数 / 赋值右侧 / WithValue / WithTimeout / WithCancel
func usesCtx(fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return false
	}
	used := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if used {
			return false
		}
		// 跳过 ctx 自身的声明位置
		if id, ok := n.(*ast.Ident); ok && id.Name == "ctx" {
			// 检查父节点,确保不是声明
			return true
		}
		// 在调用中检查 ctx 是否作为实参
		if call, ok := n.(*ast.CallExpr); ok {
			for _, arg := range call.Args {
				if id, ok := arg.(*ast.Ident); ok && id.Name == "ctx" {
					used = true
					return false
				}
			}
		}
		// 在赋值/初始化中使用 ctx
		if assign, ok := n.(*ast.AssignStmt); ok {
			for _, rhs := range assign.Rhs {
				if id, ok := rhs.(*ast.Ident); ok && id.Name == "ctx" {
					// ctx 被赋值给别的变量(说明有派生)
					used = true
					return false
				}
			}
		}
		// 在 if 条件中(检查 ctx.Err() / ctx.Done())
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "ctx" {
				used = true
				return false
			}
		}
		// 传入 context.WithValue / WithTimeout / WithCancel
		// 这些已经通过 CallExpr.Args 检查覆盖
		return true
	})
	return used
}

// isInterfaceMethod 判断函数是否属于接口声明
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

// formatSig 格式化方法签名(单行)
func formatSig(fn *ast.FuncDecl) string {
	var sb strings.Builder
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		sb.WriteString("(")
		sb.WriteString(formatExpr(fn.Recv.List[0].Type))
		sb.WriteString(") ")
	}
	sb.WriteString(fn.Name.Name)
	sb.WriteString("(")
	if fn.Type.Params != nil {
		sb.WriteString(formatFieldList(fn.Type.Params))
	}
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		sb.WriteString(") (")
		sb.WriteString(formatFieldList(fn.Type.Results))
	}
	sb.WriteString(")")
	return sb.String()
}

func formatExpr(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + formatExpr(t.X)
	case *ast.SelectorExpr:
		return formatExpr(t.X) + "." + t.Sel.Name
	}
	return "?"
}

func formatFieldList(fl *ast.FieldList) string {
	var parts []string
	for _, field := range fl.List {
		typ := formatExpr(field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, typ)
		} else {
			for _, n := range field.Names {
				parts = append(parts, n.Name+" "+typ)
			}
		}
	}
	return strings.Join(parts, ", ")
}
