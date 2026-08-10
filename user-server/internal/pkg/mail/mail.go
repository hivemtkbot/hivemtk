package mail

import (
	"crypto/tls"
	"fmt"
	"strings"

	"gopkg.in/gomail.v2"
)

type Config struct {
	Host     string // 自动推断无需赋值
	Port     int    // 自动推断无需赋值
	From     string `json:"from"`     // 发件邮箱
	Password string `json:"password"` // SMTP授权码
	SSL      bool   `json:"ssl"`      // 是否启用SSL
}

func SendMail(cfg Config, to []string, subject, body string, isHTML bool) error {
	// 自动推断SMTP配置
	if cfg.Host == "" || cfg.Port == 0 {
		autoConfig(&cfg)
	}

	m := gomail.NewMessage()
	m.SetHeader("From", cfg.From)
	m.SetHeader("To", to...)
	m.SetHeader("Subject", subject)

	// 设置内容类型
	contentType := "text/plain"
	if isHTML {
		contentType = "text/html"
	}
	m.SetBody(contentType, body)

	// 添加附件
	// for _, file := range attachments {
	// 	m.Attach(file)
	// }

	// 创建带超时的连接池
	d := &gomail.Dialer{
		Host:      cfg.Host,
		Port:      cfg.Port,
		Username:  cfg.From,
		Password:  cfg.Password,
		SSL:       cfg.SSL,
		TLSConfig: &tls.Config{ServerName: cfg.Host},
	}

	// 测试连接可用性
	if conn, err := d.Dial(); err == nil {
		conn.Close()
	} else {
		return fmt.Errorf("SMTP连接测试失败: %v", err)
	}

	return d.DialAndSend(m)
}

// 自动配置SMTP服务器参数
func autoConfig(cfg *Config) {
	domain := strings.Split(cfg.From, "@")

	// 主机名推断
	switch {
	case len(domain) > 1 && strings.Contains(domain[1], "qq.com"):
		cfg.Host = "smtp.qq.com"
	case len(domain) > 1 && strings.Contains(domain[1], "163.com"):
		cfg.Host = "smtp.163.com"
	case len(domain) > 1 && strings.Contains(domain[1], "126.com"):
		cfg.Host = "smtp.126.com"
	case len(domain) > 1 && strings.Contains(domain[1], "yeah.net"):
		cfg.Host = "smtp.yeah.net"
	case len(domain) > 1 && strings.Contains(domain[1], "sina.com"):
		cfg.Host = "smtp.sina.com"
	case len(domain) > 1 && strings.Contains(domain[1], "139.com"):
		cfg.Host = "smtp.139.com"
	case len(domain) > 1 && strings.Contains(domain[1], "gmail.com"):
		cfg.Host = "smtp.gmail.com"
	case len(domain) > 1 && (strings.Contains(domain[1], "outlook.com") || strings.Contains(domain[1], "hotmail.com")):
		cfg.Host = "smtp.live.com"
	case len(domain) > 1 && strings.Contains(domain[1], "yahoo.com"):
		cfg.Host = "smtp.mail.yahoo.com"
	case len(domain) > 1 && strings.Contains(domain[1], "aol.com"):
		cfg.Host = "smtp.aol.com"
	case len(domain) > 1 && strings.Contains(domain[1], "gmx.com"):
		cfg.Host = "smtp.gmx.com"
	default:
		cfg.Host = ""
	}

	// 端口与加密策略
	switch cfg.Host {
	case "smtp.gmail.com", "smtp.live.com":
		cfg.Port = 587 // 强制STARTTLS
		cfg.SSL = false
	case "":
		cfg.Port = 587 // 默认安全端口
	default:
		cfg.Port = 465 // 标准SSL端口
		cfg.SSL = true
	}
}
