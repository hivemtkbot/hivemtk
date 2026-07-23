// Package service 知识库子域 —— 安全/输入校验
//
// 2026-07-23 五层架构治理(二轮):从 knowledge_service.go 拆出
// URL 校验(SSRF 防护)+ HTML 标签剥离 + 段落提取等安全相关函数。
// 单一职责:所有"防止恶意输入伤害系统"的工具集中放在此文件。
//
// 2026-07-23 五层架构治理(三轮):通用实现已外迁至
// internal/pkg/utils/{url,text},本文件保留为薄包装以维持外部兼容。
package service

import (
	"context"

	"marketing/internal/pkg/utils/text"
	urlutil "marketing/internal/pkg/utils/url"
)

// validateURL URL 校验(含 SSRF 防护)
//
// 防护策略:仅允许 http/https 协议,并对解析后的所有 IP 做内网/保留地址拦截。
// 注:DNS 重绑定(TOCTOU)无法靠单次解析完全消除,生产环境应配合出口防火墙 /
// 专用 egress proxy;此处拦截已覆盖绝大多数 SSRF 利用场景(如 169.254.169.254 元数据)。
func validateURL(rawURL string) error {
	return urlutil.ValidateURL(context.Background(), rawURL)
}

// stripHTML 简单 HTML 标签剥离
func stripHTML(html string) string {
	return text.StripHTML(html)
}

// stripBetween 移除 start..end(含)之间所有成对出现的段落
func stripBetween(s, start, end string) string {
	return text.StripBetween(s, start, end)
}
