// Package main 提供 importfaq 命令行工具
//
// 设计依据: AI 智能体性能优化
//
// 职责:
//   - 读取 faq_seed.json (由 extract_faq.py 生成)
//   - 批量 upsert 到 faq_entries 表 (按 question 唯一性判断)
//   - 跳过已存在的条目, 仅新增缺失条目
//   - 输出导入统计 (OK / SKIP / FAIL)
//
// 使用:
//
//	cd hivemtk/user-server
//	go run cmd/importfaq/main.go -input ../scripts/faq_seed.json
//	go run cmd/importfaq/main.go -input ../scripts/faq_seed.json -dry-run
//
// 输出示例:
//
//	[OK] 韵达发货吗 (id=1)
//	[SKIP] 可以优惠价吗 (exists, id=2)
//	...
//	✅ Imported FAQ: ok=30 skip=20 fail=0
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lib/pq"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

type faqSeed struct {
	Question   string   `json:"question"`
	Answer     string   `json:"answer"`
	Keywords   []string `json:"keywords"`
	Category   string   `json:"category"`
	Intent     string   `json:"intent"`
	Confidence float64  `json:"confidence"`
	HitCount   int64    `json:"hit_count"`
	Enabled    *bool    `json:"enabled"`
}

func main() {
	input := flag.String("input", "../scripts/faq_seed.json", "FAQ seed JSON path")
	dryRun := flag.Bool("dry-run", false, "仅打印, 不实际写入 DB")
	flag.Parse()

	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 读取 %s 失败: %v\n", *input, err)
		os.Exit(1)
	}
	var seeds []faqSeed
	if err := json.Unmarshal(raw, &seeds); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 解析 JSON 失败: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Printf("[DRY-RUN] 将处理 %d 条 FAQ\n", len(seeds))
		for _, s := range seeds {
			fmt.Printf("  - [%s] %s\n", s.Intent, s.Question)
		}
		return
	}

	gdb := db.GetDB()
	if gdb == nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 连接 DB 失败 (db.GetDB() 返回 nil)\n")
		os.Exit(1)
	}

	var okCount, skipCount, failCount int
	for _, s := range seeds {
		// 查重: 按 question
		var existing model.FAQEntry
		err := gdb.Where("question = ?", s.Question).First(&existing).Error
		if err == nil {
			fmt.Printf("[SKIP] %s (exists, id=%d)\n", s.Question, existing.ID)
			skipCount++
			continue
		}

		// 构造模型
		entry := &model.FAQEntry{
			Question:   s.Question,
			Answer:     s.Answer,
			Keywords:   pq.StringArray(s.Keywords),
			Category:   s.Category,
			Intent:     s.Intent,
			Confidence: s.Confidence,
			HitCount:   s.HitCount,
			Enabled:    s.Enabled,
		}
		if entry.Enabled == nil {
			t := true
			entry.Enabled = &t
		}
		if err := gdb.Create(entry).Error; err != nil {
			fmt.Fprintf(os.Stderr, "[FAIL] %s: %v\n", s.Question, err)
			failCount++
			continue
		}
		fmt.Printf("[OK] %s (id=%d)\n", s.Question, entry.ID)
		okCount++
	}

	fmt.Println()
	fmt.Printf("✅ Imported FAQ: ok=%d skip=%d fail=%d\n", okCount, skipCount, failCount)
	if failCount > 0 {
		os.Exit(1)
	}
}
