package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// escapeJSON 转义 JSON 字符串值(避免特殊字符破坏 JSON)
func escapeJSON(s string) string {
	b, _ := jsonMarshalString(s)
	return b
}

// jsonMarshalString 复用 encoding/json
func jsonMarshalString(s string) (string, error) {
	// 使用 strconv.Quote 等价实现(避免循环引用)
	// 实际项目中可改为 json.Marshal
	const hexDigits = "0123456789abcdef"
	var buf strings.Builder
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"', '\\':
			buf.WriteByte('\\')
			buf.WriteByte(c)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if c < 0x20 {
				buf.WriteString(`\u00`)
				buf.WriteByte(hexDigits[c>>4])
				buf.WriteByte(hexDigits[c&0xF])
			} else {
				// 普通字符(含中英文/Unicode)原样写入
				buf.WriteByte(c)
			}
		}
	}
	buf.WriteByte('"')
	return buf.String(), nil
}

// randomNonce 生成阿里云签名用的随机数
func randomNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// specialURLEncode 阿里云 v1 API 要求的 URL 编码
// 规则:
//   - 保留字符 A-Z a-z 0-9 - _ . ~ 原样保留
//   - 空格编码为 +
//   - 其他字符：先将 UTF-8 字节按 ISO-8859-1 还原为 Unicode 码点,再以 UTF-8 编码,最后 %XX 大写
//     （这是阿里云 v1 签名约定的"SortedQueryStringEncoding"，与新版 v3 标准的 percentEncode 区分）
func specialURLEncode(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			buf.WriteByte(c)
		} else if c == ' ' {
			buf.WriteByte('+')
		} else {
			// 阿里云 v1 规则:把每个字节视为 Latin-1 字符,转换为 Unicode 码点后,
			// 再用 UTF-8 编码,然后百分号编码(大写)
			// 例如 "中"(U+4E2D) 的 UTF-8 字节为 E4 B8 AD,
			// 被当作 Latin-1 字符(码点 0xE4/0xB8/0xAD),再 UTF-8 编码得到 C3 A4 C2 B8 C2 AD
			r := rune(c)
			encoded := []byte(string(r))
			for _, b := range encoded {
				fmt.Fprintf(&buf, "%%%02X", b)
			}
		}
	}
	return buf.String()
}

// percentEncode 阿里云 v3 API 用的 percent-encode(更严格的规则)
func percentEncode(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			buf.WriteByte(c)
		} else {
			fmt.Fprintf(&buf, "%%%02X", c)
		}
	}
	return buf.String()
}

// signAliyun 计算阿里云 v1 API 签名(HMAC-SHA1)
func signAliyun(params url.Values, accessKeySecret string) string {
	// 1. 排序参数名
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "Signature" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 2. 拼接 k1=v1&k2=v2 (使用 specialURLEncode)
	var query strings.Builder
	for i, k := range keys {
		if i > 0 {
			query.WriteByte('&')
		}
		query.WriteString(specialURLEncode(k))
		query.WriteByte('=')
		query.WriteString(specialURLEncode(params.Get(k)))
	}

	// 3. StringToSign = HTTPMethod + "&" + specialURLEncode("/") + "&" + specialURLEncode(sortedQuery)
	stringToSign := "GET" + "&" + specialURLEncode("/") + "&" + specialURLEncode(query.String())

	// 4. HMAC-SHA1
	mac := hmac.New(sha1.New, []byte(accessKeySecret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// sha256Hex 返回字符串的 sha256 hex
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// hmacSHA256 HMAC-SHA256
func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// buildWSSE 构造华为云 X-WSSE 头
func buildWSSE(appKey, appSecret string) string {
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	nonce := randomNonce()
	h := sha256.New()
	h.Write([]byte(nonce + now + appSecret))
	passwordDigest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return `UsernameToken Username="` + appKey + `", PasswordDigest="` + passwordDigest + `", Nonce="` + nonce + `", Created="` + now + `"`
}
