package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	files := []string{
		"internal/service/auth_service_test.go",
		"internal/middleware/audit_test.go",
	}
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Println("read", p, err)
			continue
		}
		s := string(b)
		n := strings.Count(s, "\\t")
		s = strings.ReplaceAll(s, "\\t", "\t")
		if err := os.WriteFile(p, []byte(s), 0644); err != nil {
			fmt.Println("write", p, err)
			continue
		}
		fmt.Printf("%s: replaced %d\n", p, n)
	}
}
