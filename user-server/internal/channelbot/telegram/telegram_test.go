package telegram

import (
	"strings"
	"testing"
)

// TestMarkdownToTelegramHTML 锁定 AI 生成的 Markdown 渲染行为：
// 粗体/斜体/行内代码/链接需正确转换为 Telegram HTML，且 < > & 不能原样泄漏。
func TestMarkdownToTelegramHTML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"粗体", "**重要**通知", "<b>重要</b>通知"},
		{"斜体", "这是 *强调* 内容", "这是 <i>强调</i> 内容"},
		{"行内代码", "运行 `make up` 启动", "运行 <code>make up</code> 启动"},
		{"链接", "详情见 [文档](https://example.com/a?x=1&y=2)", `详情见 <a href="https://example.com/a?x=1&amp;y=2">文档</a>`},
		{"HTML 转义", "价格 < 100 且 a & b", "价格 &lt; 100 且 a &amp; b"},
		{"未闭合不加粗", "普通 ** 星号保留", "普通 ** 星号保留"},
		{"纯文本", "你好世界", "你好世界"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := markdownToTelegramHTML(c.in)
			if got != c.want {
				t.Fatalf("输入 %q\n期望 %q\n实际 %q", c.in, c.want, got)
			}
		})
	}
}

// TestMarkdownToTelegramHTML_NoRawTags 确保转换结果不会把字面 < > & 漏给 Telegram
func TestMarkdownToTelegramHTML_NoRawTags(t *testing.T) {
	got := markdownToTelegramHTML("a < b > c & d **x**")
	if strings.Contains(got, "< b") || strings.Contains(got, "> c") || strings.Contains(got, " & d") {
		t.Fatalf("出现未转义的特殊字符: %q", got)
	}
	if !strings.Contains(got, "<b>x</b>") {
		t.Fatalf("粗体未转换: %q", got)
	}
}
