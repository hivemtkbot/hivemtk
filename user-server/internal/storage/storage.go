// Package storage 统一存储抽象层 —— 私有化部署零云依赖，改配置即可切换云厂商。
//
// 五层架构位置：独立基础设施包，被 service 层调用。
//
// 目录规范：
//
//	./uploads/
//	├── attachments/       对话附件（按用户隔离）
//	├── materials/         用户端素材库
//	├── covers/            资产包封面 + 缩略图
//	├── avatars/           用户头像
//	└── temp/              临时文件（定期清理）
package storage

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"time"
)

// Driver 存储驱动接口 —— 所有 provider（local/minio/oss/cos/s3）必须实现。
type Driver interface {
	// 上传 multipart 文件，返回访问 URL 和存储内部路径
	UploadMultipart(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (url string, storagePath string, err error)
	// 上传字节流，调用方负责关闭 reader
	UploadReader(ctx context.Context, reader io.Reader, size int64, folder string, filename string) (url string, storagePath string, err error)
	// 下载文件，返回可读流
	Download(ctx context.Context, storagePath string) (io.ReadCloser, error)
	// 删除文件
	Delete(ctx context.Context, storagePath string) error
	// 获取公开访问 URL（local 返回 /files/xxx，云存储返回完整 CDN URL）
	PublicURL(storagePath string) string
	// 生成预签名上传 URL（返回 upload_url + object_key + expires）
	SignUploadURL(ctx context.Context, folder string, filename string, contentType string, expire time.Duration) (uploadURL string, objectKey string, err error)
	// 检查文件是否存在
	Exists(ctx context.Context, storagePath string) (bool, error)
	// 获取驱动类型标识
	Type() string
}

// ErrNoDefaultStorage 没有默认存储配置
var ErrNoDefaultStorage = errors.New("no default storage configured")

// ErrUnsupportedProvider 不支持的 provider
var ErrUnsupportedProvider = errors.New("unsupported storage provider")
