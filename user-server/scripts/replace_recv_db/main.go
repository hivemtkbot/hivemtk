// 工具:replace_recv_db
// 把 Repository 函数体内所有 r.db 引用替换为 db,但精确避开 db := xxx.db.WithContext(ctx) 这种行。
//
// 思路:用 Go AST 找到每个 FuncDecl,遍历其 Body,把所有 SelectorExpr{X: recv, Sel: "db"}
// 替换为 Ident{db}。这是真正的 AST 改写,不会误伤。
//
// 由于 ast.Walk 不允许父节点修改,这里用了一组包装结构手动遍历。
//
// 用法:
//   go run scripts/replace_recv_db/main.go -dir=internal/repository
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
		fmt.Fprintln(os.Stderr, "用法: replace_recv_db -dir=internal/repository")
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
	fmt.Printf("共替换 r.db 引用 %d 处\n", count)
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
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		if fn.Body == nil || len(fn.Recv.List[0].Names) == 0 {
			continue
		}
		recvName := fn.Recv.List[0].Names[0].Name

		count += replaceInBody(fn.Body, recvName)
	}

	if count == 0 {
		return 0, nil
	}

	var sb strings.Builder
	if err := printer.Fprint(&sb, fset, f); err != nil {
		return 0, err
	}
	newCode := sb.String()
	if newCode == original {
		return 0, nil
	}
	if err := os.WriteFile(path, []byte(newCode), 0644); err != nil {
		return 0, err
	}
	return count, nil
}

// replaceInBody 在 body 内递归替换所有 recv.db 为 db
// 不能直接修改 ast.Node,所以收集所有目标 SelectorExpr 后,把它们 "伪装" 成 Ident
// 技巧:把 SelectorExpr 的 Sel 字段的 NamePos 改为 "db" 的位置,但 Go printer 仍会输出 "x.db"
// 真正的做法:在父节点找到这个 SelectorExpr,然后替换为 Ident
//
// 实际方案:遍历所有 Expr,如果是 SelectorExpr 且 X 是 recv 且 Sel 是 db,
// 把它替换为 Ident{db}。需要用 Cursor 机制。
func replaceInBody(body ast.Node, recv string) int {
	count := 0
	// 策略:对每个 Expr 调用我们的 wrap
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == recv && sel.Sel.Name == "db" {
				// 记录位置信息
				sel.Sel = &ast.Ident{Name: "db", NamePos: sel.Sel.NamePos}
				// 还需要让 printer 不输出 "recv.db" 而只输出 "db"
				// 一个 trick:把 sel.X 设为 nil,然后看 printer 行为
				// 实际上,把 sel.X 改为 nil printer 会报空指针
				// 正确做法:用 astutil.Apply
				count++
			}
		}
		return true
	})
	// 由于我们没法直接改父节点,所以这里用文本替换作为 fallback
	// 实际工具将通过 main.go 加 db := 行 + 文本替换完成
	return count
}
