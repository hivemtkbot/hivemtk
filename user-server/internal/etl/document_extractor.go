package etl

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
	"golang.org/x/net/html"
)

// ExtractText 从原始文档字节中提取纯文本，按文件扩展名选择解析器。
// 这是「整本文档导入 → 切片 → 向量化 → 入库」链路的第一步：
// 必须先拿到可读文本，否则后续切片/向量/检索都是二进制乱码。
//
// 支持的格式：
//   - 纯文本类(.txt/.md/.csv/.json/.log/.xml/.yaml 等)：直接作为 UTF-8 文本返回
//   - HTML(.html/.htm)：剥离标签，保留段落/换行结构
//   - DOCX(.docx)：解压后解析 word/document.xml（标准库实现，无需外部依赖）
//   - PDF(.pdf)：使用 ledongthuc/pdf 逐页提取文本
//   - 其它/未知：退化为 UTF-8 文本（尽量不丢内容）
func ExtractText(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md", ".markdown", ".csv", ".tsv", ".json", ".log", ".xml", ".yml", ".yaml", ".text":
		return string(data), nil
	case ".html", ".htm":
		return extractHTMLText(data), nil
	case ".docx":
		return extractDocx(data)
	case ".pdf":
		return extractPDF(data)
	case ".doc":
		// 旧版 .doc 为私有二进制格式，Go 无零依赖可靠解析方案，明确报错引导转换。
		return "", fmt.Errorf("不支持的旧版 .doc 二进制格式，请先转换为 .docx 或 .pdf 后再导入")
	default:
		// 未知扩展名：退化为 UTF-8 文本，避免直接丢失内容。
		return string(data), nil
	}
}

// extractHTMLText 去除 HTML 标签，保留基本换行结构。
func extractHTMLText(data []byte) string {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return string(data)
	}
	var buf strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				buf.WriteString(text)
				buf.WriteString(" ")
			}
		} else if n.Type == html.ElementNode {
			switch n.Data {
			case "br", "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6":
				buf.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return collapseSpaces(buf.String())
}

// extractDocx 解析 Office Open XML(.docx)。docx 本质是一个 zip 包，
// 正文文本位于 word/document.xml 的 <w:t> 节点中，<w:p> 表示段落。
func extractDocx(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("打开 docx(zip) 失败: %w", err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			raw, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return "", err
			}
			return extractDocxBody(raw)
		}
	}
	return "", fmt.Errorf("docx 中未找到 word/document.xml")
}

// extractDocxBody 流式解析 document.xml，按文档顺序收集 <w:t> 文本，
// 遇到段落(<w:p>)时插入换行，尽量保留可读结构。
func extractDocxBody(raw []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	decoder.Strict = false
	var sb strings.Builder
	inText := false
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "p":
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
			case "tab":
				sb.WriteString("\t")
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				sb.WriteString(string(t))
			}
		}
	}
	return collapseSpaces(sb.String()), nil
}

// extractPDF 逐页提取 PDF 文本。
func extractPDF(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("打开 pdf 失败: %w", err)
	}
	var sb strings.Builder
	pages := reader.NumPage()
	for i := 1; i <= pages; i++ {
		page := reader.Page(i)
		// VanityErr 在某些版本的 pdf 库不存在；忽略以保持向后兼容
		_ = i
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		if strings.TrimSpace(text) != "" {
			sb.WriteString(text)
			sb.WriteString("\n")
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("pdf 未提取到任何文本内容（可能为扫描件/图片型 PDF）")
	}
	return collapseSpaces(sb.String()), nil
}

// collapseSpaces 折叠多余空白，保留段落换行。
func collapseSpaces(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
