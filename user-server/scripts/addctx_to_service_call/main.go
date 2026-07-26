// 工具:addctx_to_service_call
// 扫描 service 目录,找到 Service 方法中调用 Repository 字段或本 Service 中其他方法的地方,
// 自动在第一个实参前插入 ctx(如果被调方法已加 ctx)。
//
// 用法:
//
//	go run scripts/addctx_to_service_call/main.go -dir=internal/service
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

	// 收集本文件中所有 Service 字段
	fieldMap := collectServiceFields(f)

	// 收集本文件/包内所有需要 ctx 的方法名(以 Service 结尾的 receiver 上的方法)
	ctxMethodNames := collectCtxMethodNames(f)

	fixes := 0
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Type == nil || fn.Type.Params == nil || fn.Body == nil {
			continue
		}
		// 仅处理有 ctx context.Context 参数的方法
		if !hasContextParam(fn) {
			continue
		}

		// 找出本方法体内所有需要插入 ctx 的调用
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// sel.X 形式 1:直接是 xxxxRepo / xxxxService / registry / client / etc
			if xIdent, ok := sel.X.(*ast.Ident); ok {
				methodName := sel.Sel.Name
				if !isLikelyRepoMethod(methodName) {
					return true
				}
				// 已有 ctx
				if len(call.Args) > 0 {
					if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "ctx" {
						return true
					}
				}
				// 形式 1.1: 字段名是 xxxxRepo / xxxxService / etc
				if isRepoField(xIdent.Name) {
					insertCtx(call)
					fixes++
					return true
				}
				// 形式 1.2: sel.X 是 s / r / 已知字段,而 sel.Sel 是本 Service 中的方法
				if xIdent.Name == "s" || xIdent.Name == "r" || fieldMap[xIdent.Name] {
					if ctxMethodNames[methodName] {
						insertCtx(call)
						fixes++
					}
				}
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
				// 已有 ctx
				if len(call.Args) > 0 {
					if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "ctx" {
						return true
					}
				}
				insertCtx(call)
				fixes++
			}
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

func insertCtx(call *ast.CallExpr) {
	ctxArg := &ast.Ident{Name: "ctx"}
	newArgs := make([]ast.Expr, 0, len(call.Args)+1)
	newArgs = append(newArgs, ctxArg)
	newArgs = append(newArgs, call.Args...)
	call.Args = newArgs
}

func collectCtxMethodNames(f *ast.File) map[string]bool {
	methodMap := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Type == nil || fn.Type.Params == nil {
			return true
		}
		if fn.Recv == nil || len(fn.Recv.List) == 0 {
			return true
		}
		recvName := receiverName(fn.Recv.List[0].Type)
		if !strings.HasSuffix(recvName, "Service") && !strings.HasSuffix(recvName, "service") {
			return true
		}
		if hasContextParam(fn) {
			methodMap[fn.Name.Name] = true
		}
		return true
	})
	return methodMap
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
				if isRepoField(n) {
					fieldMap[n] = true
				}
			}
		}
		return true
	})
	return fieldMap
}

func isRepoField(name string) bool {
	suffixes := []string{
		"Repo", "repo",
		"Service", "service",
		"DB", "db",
		"DAO", "dao",
		"Registry", "registry",
		"Client", "client",
		"Manager", "manager",
		"Engine", "engine",
		"ServiceProvider", "serviceProvider",
		"Factory", "factory",
		"Provider", "provider",
		"Loader", "loader",
		"Store", "store",
		"Cache", "cache",
		"Queue", "queue",
		"Pool", "pool",
		"Handler", "handler",
		"Monitor", "monitor",
		"Tracker", "tracker",
		"Watcher", "watcher",
		"Recorder", "recorder",
		"Collector", "collector",
		"Scorer", "scorer",
		"Analyzer", "analyzer",
		"Evaluator", "evaluator",
		"Polisher", "polisher",
		"Embedder", "embedder",
		"Rerank", "rerank",
		"Searcher", "searcher",
		"Retriever", "retriever",
		"Broadcaster", "broadcaster",
		"Limiter", "limiter",
		"Uploader", "uploader",
		"Downloader", "downloader",
		"Helper", "helper",
		"Builder", "builder",
		"Dispatcher", "dispatcher",
		"Forwarder", "forwarder",
		"Speaker", "speaker",
		"Router", "router",
		"Poller", "poller",
		"Adapter", "adapter",
		"Pipeline", "pipeline",
		"Workflow", "workflow",
		"Scheduler", "scheduler",
		"Worker", "worker",
		"Executor", "executor",
		"Node", "node",
		"Graph", "graph",
		"Tree", "tree",
		"Set", "set",
		"Map", "map",
		"Matcher", "matcher",
		"Filter", "filter",
		"Sorter", "sorter",
		"Aggregator", "aggregator",
		"Calculator", "calculator",
	}
	for _, suf := range suffixes {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}

func isLikelyRepoMethod(name string) bool {
	skipMethods := map[string]bool{
		"WithContext":    true,
		"GetDB":          true,
		"SetDB":          true,
		"DB":             true,
		"String":         true,
		"Error":          true,
		"Format":         true,
		"Equal":          true,
		"IsZero":         true,
		"GoString":       true,
		"MarshalJSON":    true,
		"UnmarshalJSON":  true,
		"Valid":          true,
		"Lock":           true,
		"Unlock":         true,
		"RLock":          true,
		"RUnlock":        true,
		"Add":            true,
		"Remove":         true,
		"Has":            true,
		"Get":            true,
		"Set":            true,
		"Keys":           true,
		"Values":         true,
		"Len":            true,
		"Cap":            true,
		"Reset":          true,
		"Close":          true,
		"Load":           true,
		"Store":          true,
		"Swap":           true,
		"CompareAndSwap": true,
		"Range":          true,
		"Do":             true,
		"Done":           true,
		"Err":            true,
		"Value":          true,
		"Deadline":       true,
	}
	if skipMethods[name] {
		return false
	}
	if len(name) == 0 || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	return true
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

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverName(t.X)
	}
	return ""
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
