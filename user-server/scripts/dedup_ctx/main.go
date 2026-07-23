// dedup_ctx: 移除 Go 代码中重复的 ctx 参数
// 例: foo(ctx, ctx, x) → foo(ctx, x)
// 用法: go run dedup_ctx.go <file1> [file2 ...]
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: dedup_ctx <dir|file>...")
		os.Exit(1)
	}
	total := 0
	for _, arg := range os.Args[1:] {
		fi, err := os.Stat(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat %s: %v\n", arg, err)
			continue
		}
		if fi.IsDir() {
			filepath.Walk(arg, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				n, err := processFile(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
				}
				total += n
				return nil
			})
		} else {
			n, err := processFile(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", arg, err)
			}
			total += n
		}
	}
	fmt.Printf("deduped %d\n", total)
}

func processFile(path string) (int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return 0, err
	}

	fixed := 0
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// 查找相邻的两个 ctx 标识符
		if len(call.Args) < 2 {
			return true
		}
		newArgs := call.Args[:0]
		prevWasCtx := false
		for _, arg := range call.Args {
			if id, ok := arg.(*ast.Ident); ok && id.Name == "ctx" && prevWasCtx {
				fixed++
				continue
			}
			newArgs = append(newArgs, arg)
			if id, ok := arg.(*ast.Ident); ok && id.Name == "ctx" {
				prevWasCtx = true
			} else {
				prevWasCtx = false
			}
		}
		call.Args = newArgs
		return true
	})

	if fixed == 0 {
		return 0, nil
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, f); err != nil {
		return 0, err
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		formatted = buf.Bytes()
	}
	return fixed, os.WriteFile(path, formatted, 0644)
}
