// @title HiveMtk API
// @version 2.0.0
// @description 营销工具套件 API 文档
// @description 提供全面的营销自动化工具，包括邮件营销、短信营销、社交媒体管理等

// @contact.name API Support
// @contact.url https://github.com/marketing-tools-kit
// @contact.email support@marketingtoolskit.com

// @license.name Apache 2.0
// @license.url https://www.apache.org/licenses/LICENSE-2.0.html

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

// @externalDocs.description OpenAPI 规范
// @externalDocs.url https://swagger.io/specification/

package docs

import (
	_ "github.com/swaggo/files"
	_ "github.com/swaggo/gin-swagger"
)
