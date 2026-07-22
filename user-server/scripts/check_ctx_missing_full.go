// check_ctx_missing_full.go - 全量扫描所有含 (r *xxx) 方法签名的文件，按包分组统计
//
// 2026-07-22 方向E-预调研：
//   - 扫描 internal/repository/*.go
//   - 扫描 internal/content/repository/*.go
//   - 扫描 internal/aiagent/agent/*repository*.go
//   - 统计缺 ctx 的方法数
//
// 用法：go run scripts/check_ctx_missing_full.go
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

var excludedMethods = map[string]bool{
	"GetDB":  true,
	"WithTx": true,
	"SetDB":  true,
	"DB":     true,
}

type FileStat struct {
	Path    string
	Missing int
	Total   int
	Methods []string
}

type PkgStat struct {
	Pkg       string
	Files     int
	Total     int
	Missing   int
	FileStats []FileStat
}

func main() {
	dirs := []string{
		"internal/repository",
		"internal/content/repository",
		"internal/aiagent/agent",
		"internal/aiagent/agent/tooluse",
	}

	pkgStats := make(map[string]*PkgStat)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read dir %s failed: %v\n", dir, err)
			continue
		}
		pkg := filepath.Base(dir)
		ps, ok := pkgStats[pkg]
		if !ok {
			ps = &PkgStat{Pkg: pkg}
			pkgStats[pkg] = ps
		}

		for _, e := range entries {
			if e.IsDir() || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			missing, total, methods := scanFile(path)
			if total == 0 {
				continue
			}
			ps.Files++
			ps.Total += total
			ps.Missing += missing
			if missing > 0 {
				ps.FileStats = append(ps.FileStats, FileStat{Path: path, Missing: missing, Total: total, Methods: methods})
			}
		}
	}

	fmt.Printf("==== 全量 ctx 缺失检测（按包汇总）====\n")
	totalMiss := 0
	totalMethods := 0
	var pkgList []string
	for k := range pkgStats {
		pkgList = append(pkgList, k)
	}
	sort.Strings(pkgList)

	for _, pkg := range pkgList {
		ps := pkgStats[pkg]
		fmt.Printf("\n📦 包 %s：%d 个文件，%d/%d 方法缺 ctx\n", ps.Pkg, ps.Files, ps.Missing, ps.Total)
		totalMiss += ps.Missing
		totalMethods += ps.Total

		sort.Slice(ps.FileStats, func(i, j int) bool {
			return ps.FileStats[i].Missing > ps.FileStats[j].Missing
		})
		for _, fs := range ps.FileStats {
			if fs.Missing == 0 {
				continue
			}
			fmt.Printf("   ❌ %s: %d/%d\n", fs.Path, fs.Missing, fs.Total)
		}
	}
	fmt.Printf("\n==== 汇总 ====\n缺 ctx 方法总数：%d / %d（占比 %.1f%%）\n", totalMiss, totalMethods, float64(totalMiss)/float64(totalMethods)*100)
}

func scanFile(path string) (missing, total int, methods []string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s failed: %v\n", path, err)
		return
	}

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Recv.NumFields() == 0 {
			return true
		}
		recv := fn.Recv.List[0].Type
		if _, ok := recv.(*ast.StarExpr); !ok {
			return true
		}
		methodName := fn.Name.Name
		if excludedMethods[methodName] {
			return true
		}
		total++

		hasCtx := false
		if fn.Type.Params != nil {
			for _, p := range fn.Type.Params.List {
				if isContextType(p.Type) {
					hasCtx = true
					break
				}
			}
		}
		if !hasCtx {
			missing++
			methods = append(methods, methodName)
		}
		return true
	})
	return
}

func isContextType(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok && x.Name == "context" && e.Sel.Name == "Context" {
			return true
		}
	case *ast.Ident:
		if e.Name == "Context" {
			return true
		}
	}
	return false
}
