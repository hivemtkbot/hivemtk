package mail

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type TemplateParseMap struct {
	Name    string
	City    string
	Address string
	Account string
}

func Parse(htmlstr string, clue TemplateParseMap) string {
	fields := map[string]string{
		"name":    clue.Name,
		"city":    clue.City,
		"address": clue.Address,
		"account": clue.Account,
	}
	// 遍历 fields 替换 htmlstr 中的自定义变量
	for key, value := range fields {
		if strings.Contains(htmlstr, "{"+key+"}") {
			htmlstr = strings.ReplaceAll(htmlstr, "{"+key+"}", value)
		}
	}
	return htmlstr
}

func BuildTrace(htmlstr string, traceID uuid.UUID, websiteURL string) string {
	// 准备 image url
	image_url := fmt.Sprintf("%s/email/trace/%s", websiteURL, traceID.String())

	// 准备image
	image := fmt.Sprintf("<img src=\"%s\" style=\"width:1px; height: 1px;\" />", image_url)

	// 追加 image 到 htmlstr
	htmlstr = htmlstr + image
	return htmlstr
}
