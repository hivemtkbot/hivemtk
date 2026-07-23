// build_fix: 解析 go build 错误，针对性修复 ctx 透传问题
// 用法: go run build_fix.go
package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

func main() {
	// 1. 运行 go build 并捕获错误
	cmd := exec.Command("go", "build", "-o", "/dev/null", "./cmd/api")
	cmd.Dir = "/Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server"
	out, _ := cmd.CombinedOutput()
	errors := string(out)

	// 2. 解析错误
	// 模式: file:line:col: not enough arguments in call to X
	//        have (...)
	//        want (context.Context, ...)
	reErr := regexp.MustCompile(`^([^:]+):(\d+):(\d+): (.+)$`)
	scanner := bufio.NewScanner(strings.NewReader(errors))
	type Fix struct {
		File    string
		Line    int
		Col     int
		Message string
		Have    string
		Want    string
	}
	var fixes []Fix
	var current *Fix
	for scanner.Scan() {
		line := scanner.Text()
		if m := reErr.FindStringSubmatch(line); m != nil {
			if current != nil {
				fixes = append(fixes, *current)
			}
			lineNum := 0
			fmt.Sscanf(m[2], "%d", &lineNum)
			col := 0
			fmt.Sscanf(m[3], "%d", &col)
			current = &Fix{File: m[1], Line: lineNum, Col: col, Message: m[4]}
		} else if strings.HasPrefix(line, "\t") && current != nil {
			content := strings.TrimPrefix(line, "\t")
			if strings.HasPrefix(content, "have ") {
				current.Have = content
			} else if strings.HasPrefix(content, "want ") {
				current.Want = content
			}
		}
	}
	if current != nil {
		fixes = append(fixes, *current)
	}

	fmt.Printf("found %d errors\n", len(fixes))
	for _, f := range fixes {
		fmt.Printf("  %s:%d %s | have=%s | want=%s\n", f.File, f.Line, f.Message, f.Have, f.Want)
	}

	// 3. 按文件分组
	byFile := make(map[string][]Fix)
	for _, f := range fixes {
		byFile[f.File] = append(byFile[f.File], f)
	}

	// 4. 输出每个文件的错误统计
	for file, fs := range byFile {
		fmt.Printf("\n%s: %d errors\n", file, len(fs))
	}
}
