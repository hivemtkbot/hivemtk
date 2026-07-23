// 智能批量为 service 文件中方法调用添加 ctx 参数
// 检测目标方法的第一个参数是否已经是 ctx，避免重复
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

// 已知需要 ctx 的方法名（小写方法名 → 是否需要）
var serviceMethods = map[string]bool{
	// AIAgentService
	"Create": true, "GetByID": true, "Update": true, "UpdateStatus": true, "Delete": true, "List": true, "ListEnabled": true, "TestAgent": true, "LoadContext": true,
	// AccountService
	"CreateAccount": true, "GetAccount": true, "GetAccountList": true, "UpdateAccount": true, "UpdateAccountStatusById": true, "UpdateAccountTgNameById": true,
	// ClueService
	"BatchImportClues": true,
	// AnomalyLoginDetector
	"DetectAndAlert": true, "ListAlerts": true, "ListLoginEvents": true, "ResolveAlert": true, "IgnoreAlert": true, "writeInboxNotification": true, "sendEmailAlert": true, "writeAuditLog": true,
	// LoginRiskService
	"Evaluate": true, "ListSecurityAlerts": true, "ResolveSecurityAlert": true, "IgnoreSecurityAlert": true,
	// AuthService
	"loginWithUser": true, "toUserResponse": true, "cleanupExpiredForgotTokens": true, "RefreshToken": true,
	// MFAService
	"IsMFAEnabled": true, "IssueTempToken": true,
	// PasswordPolicyService
	"ValidatePassword": true, "RecordPasswordHistory": true,
	// AutoReplyService
	"isWithinTimeRange": true, "SaveRule": true, "UpsertAccount": true,
	// ChannelAgentBindingService, CustomerServiceAgentService
	"CreateChannelBinding": true, "UpdateChannelBindingFromJSON": true, "CreateCSAgentMount": true, "UpdateCSAgentMountFromJSON": true, "CreateAIAgent": true, "UpdateAIAgentFromJSON": true,
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: smart_fix <dir1> [dir2...]")
		os.Exit(1)
	}

	totalFixed := 0
	for _, root := range os.Args[1:] {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			n, err := processFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error in %s: %v\n", path, err)
			} else if n > 0 {
				totalFixed += n
				fmt.Printf("fixed %d in %s\n", n, path)
			}
			return nil
		})
	}
	fmt.Printf("total: %d\n", totalFixed)
}

func processFile(path string) (int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return 0, err
	}

	// 收集当前文件已定义的方法
	definedMethods := make(map[string]bool)
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		definedMethods[fd.Name.Name] = true
	}

	fixed := 0
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// 检查是否需要修复
		if !serviceMethods[sel.Sel.Name] {
			return true
		}

		// 检查是否已经有 ctx 作为第一个参数
		if len(call.Args) > 0 {
			if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "ctx" {
				return true
			}
		}

		// 插入 ctx
		ctxIdent := &ast.Ident{Name: "ctx"}
		newArgs := make([]ast.Expr, 0, len(call.Args)+1)
		newArgs = append(newArgs, ctxIdent)
		newArgs = append(newArgs, call.Args...)
		call.Args = newArgs
		fixed++
		return true
	})

	if fixed == 0 {
		return 0, nil
	}

	// 写回
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, f); err != nil {
		return 0, err
	}
	// 格式化
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// 即使格式化失败也写
		formatted = buf.Bytes()
	}

	return fixed, os.WriteFile(path, formatted, 0644)
}
