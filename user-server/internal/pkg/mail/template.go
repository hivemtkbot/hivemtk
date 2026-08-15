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
	for key, value := range fields {
		if strings.Contains(htmlstr, "{"+key+"}") {
			htmlstr = strings.ReplaceAll(htmlstr, "{"+key+"}", value)
		}
	}
	return htmlstr
}

func BuildTrace(htmlstr string, traceID uuid.UUID, websiteURL string) string {
	image_url := fmt.Sprintf("%s/email/trace/%s", websiteURL, traceID.String())

	image := fmt.Sprintf("<img src=\"%s\" style=\"width:1px; height: 1px;\" />", image_url)

	htmlstr = htmlstr + image
	return htmlstr
}

