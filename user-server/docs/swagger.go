// @title HiveMtk API
// @version 2.0.0
// @description 私域营销 AI 操作系统 API 文档
// @description 提供全面的私域营销与 AI 智能体能力，包括 94 个业务模块、三级 RAG 检索、ReAct 智能体、9 触达渠道、CDP 客户数据平台等
// @description 完全离线部署，数据零出域

// @contact.name HiveMtk 维护团队
// @contact.url https://github.com/hivemtk
// @contact.email support@hivemtk.io
// @contact.name 文档站点
// @contact.url https://hivemtk.io/docs

// @license.name AGPL-3.0
// @license.url https://www.gnu.org/licenses/agpl-3.0.html

// @host localhost:8204
// @BasePath /api

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT Token 认证，格式：Bearer {token}

// @securityDefinitions.apikey APIKeyAuth
// @in header
// @name X-API-KEY
// @description API Key 认证

// @securityDefinitions.apikey MerchantAuth
// @in header
// @name X-Merchant-Key
// @description 商户 HMAC 鉴权（平台端商户 API 使用），需配合 X-Timestamp + X-Signature

// @externalDocs.description HiveMtk 完整文档
// @externalDocs.url https://hivemtk.io/docs

// @x-extension-openmetadatas {"product":"HiveMtk","type":"private-domain-marketing","license":"AGPL-3.0","selfHosted":true}

package docs

import (
	_ "github.com/swaggo/files"
	_ "github.com/swaggo/gin-swagger"
)
