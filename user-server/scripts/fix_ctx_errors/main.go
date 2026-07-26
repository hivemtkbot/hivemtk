// 工具:fix_ctx_errors
// 严格修复 ctx 透传过程中的"误加 ctx"编译错误:
//  1. sync.RWMutex / sync.Mutex 的方法(Lock/Unlock/RLock/RUnlock)
//  2. WebSocketBroadcaster.PushReviewItem(无 ctx)
//  3. EmbedderInterface 的方法(EmbedText/EmbedQuery/GetDimension,无 ctx)
//
// 不处理 gorm.DB 字段(由 fix_sdb_withctx 工具保证 WithContext 链式)。
//
// 用法:
//
//	go run scripts/fix_ctx_errors/main.go
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

// 已知不需要 ctx 的"误加 ctx"规则
type removeCtxRule struct {
	fieldName   string   // 字段名
	methodNames []string // 不需要 ctx 的方法名
}

var removeCtxRules = []removeCtxRule{
	// sync.RWMutex / sync.Mutex 的方法(用规则 2 也覆盖)
	{"mu", []string{"Lock", "Unlock", "RLock", "RUnlock"}},
	{"mutex", []string{"Lock", "Unlock", "RLock", "RUnlock"}},
	{"rwMutex", []string{"Lock", "Unlock", "RLock", "RUnlock"}},
	{"lock", []string{"Lock", "Unlock", "RLock", "RUnlock"}},
	// WebSocketBroadcaster 接口
	{"wsBroadcaster", []string{"PushReviewItem", "Broadcast", "Push", "Send"}},
	// Embedder 接口
	{"embedder", []string{"EmbedText", "EmbedQuery", "GetDimension"}},
	{"embed", []string{"EmbedText", "EmbedQuery", "GetDimension"}},
	{"embedding", []string{"EmbedText", "EmbedQuery", "GetDimension"}},
	// RAGEngine 部分方法无 ctx
	{"ragEngine", []string{"UpdateConfig", "Stats"}},
	// LLMService 部分方法无 ctx
	{"llmService", []string{"GetDefaultConfig", "ValidateConfig"}},
	// RAGThreeTier 部分方法无 ctx
	{"threeTier", []string{"Stats"}},
	// MigrationRegistry 部分方法无 ctx
	{"registry", []string{"Get", "GetPending", "Add", "Remove", "List"}},
}

func main() {
	dirs := flag.String("dirs", "internal", "目标目录,逗号分隔")
	flag.Parse()

	totalFixes := 0
	for _, d := range strings.Split(*dirs, ",") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		n, err := walkDir(d)
		if err != nil {
			fmt.Fprintln(os.Stderr, "WALK ERR", d, err)
			continue
		}
		totalFixes += n
	}
	fmt.Printf("共修复 %d 处\n", totalFixes)
}

func walkDir(root string) (int, error) {
	totalFixes := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(path, "/scripts/") {
			return nil
		}
		n, e := fixFile(path)
		if e == nil && n > 0 {
			totalFixes += n
			fmt.Printf("  ✓ %s 修复 %d 处\n", path, n)
		}
		return nil
	})
	return totalFixes, err
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

	// 收集本文件所有结构体字段名 -> 类型字符串
	fieldTypes := collectFieldTypes(f)

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

		fieldName := extractFieldName(sel.X)
		if fieldName == "" {
			return true
		}

		methodName := sel.Sel.Name

		// 规则 1: 字段名命中 removeCtxRules
		matchedRule := false
		for _, r := range removeCtxRules {
			if fieldName == r.fieldName {
				for _, m := range r.methodNames {
					if methodName == m {
						if removeFirstArgIfCtx(call) {
							fixes++
						}
						matchedRule = true
						break
					}
				}
			}
		}
		if matchedRule {
			return true
		}

		// 规则 2: 字段类型是 sync.RWMutex / sync.Mutex
		if isMutexField(fieldName, fieldTypes) {
			if methodName == "Lock" || methodName == "Unlock" || methodName == "RLock" || methodName == "RUnlock" {
				if removeFirstArgIfCtx(call) {
					fixes++
				}
				return true
			}
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

// collectFieldTypes 收集 struct 字段名 -> 类型字符串映射
func collectFieldTypes(f *ast.File) map[string]string {
	m := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range st.Fields.List {
			typeStr := exprToString(field.Type)
			for _, name := range field.Names {
				if name != nil {
					m[name.Name] = typeStr
				}
			}
			if len(field.Names) == 0 {
				parts := strings.Split(typeStr, ".")
				alias := parts[len(parts)-1]
				m[alias] = typeStr
			}
		}
		return true
	})
	return m
}

func isMutexField(name string, types map[string]string) bool {
	t, ok := types[name]
	if !ok {
		return false
	}
	return t == "sync.RWMutex" || t == "sync.Mutex" || t == "*sync.RWMutex" || t == "*sync.Mutex"
}

// extractFieldName 从表达式中提取最右侧的字段名
//   - *ast.Ident → 名字
//   - *ast.SelectorExpr → 右侧字段名
//   - 其他 → ""
func extractFieldName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}

// removeFirstArgIfCtx 当第一个实参是名为 ctx 的 ident 时,移除它
func removeFirstArgIfCtx(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	first := call.Args[0]
	id, ok := first.(*ast.Ident)
	if !ok || id.Name != "ctx" {
		return false
	}
	call.Args = call.Args[1:]
	return true
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
