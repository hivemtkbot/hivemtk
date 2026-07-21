//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"

	trumail "github.com/sdwolfe32/trumail/verifier"
)

func main() {
	// 从环境变量读取配置
	smtpHost := os.Getenv("EMAIL_163_SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "smtp.163.com"
	}

	emailUser := os.Getenv("EMAIL_163_USER")
	if emailUser == "" {
		emailUser = "myloveisphp@163.com"
	}

	qqEmail := os.Getenv("QQ_EMAIL")
	if qqEmail == "" {
		qqNumber := os.Getenv("QQ_NUMBER")
		if qqNumber != "" {
			qqEmail = qqNumber + "@qq.com"
		} else {
			qqEmail = "1036698712@qq.com"
		}
	}

	v := trumail.NewVerifier(smtpHost, emailUser)
	// Validate a single address
	res, err := v.Verify(qqEmail)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res.FullInbox)
}
