package storage

import (
	"context"
	"fmt"
	"hivemtk-user/internal/model"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"time"
)

// Factory 从 ObsConfig 构造对应 Driver
func Factory(cfg *model.ObsConfig) (Driver, error) {
	if cfg == nil {
		return nil, ErrNoDefaultStorage
	}
	switch cfg.Provider {
	case model.ObsProviderLocal:
		return newLocalFromConfig(cfg), nil
	case model.ObsProviderAliyun, model.ObsProviderTencent, model.ObsProviderQiniu, model.ObsProviderAWS:

		return newCloudStub(cfg), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, cfg.Provider)
	}
}

func newLocalFromConfig(cfg *model.ObsConfig) Driver {
	baseDir := strings.TrimSpace(cfg.Endpoint)
	if baseDir == "" {
		baseDir = os.Getenv("STORAGE_LOCAL_BASE_DIR")
	}
	if baseDir == "" {
		baseDir = "./uploads"
	}

	publicURL := strings.TrimSpace(cfg.Domain)
	if publicURL == "" {
		publicURL = os.Getenv("STORAGE_LOCAL_PUBLIC_URL")
	}
	if publicURL == "" {
		publicURL = "/files"
	}

	return NewLocalDriver(baseDir, publicURL)
}

func newCloudStub(cfg *model.ObsConfig) Driver {
	return &cloudStub{cfg: cfg}
}

type cloudStub struct {
	cfg *model.ObsConfig
}

func (c *cloudStub) UploadMultipart(_ context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, string, error) {
	_ = file
	_ = header
	_ = folder
	return "", "", fmt.Errorf("cloud provider %s SDK not wired yet; fall back to local storage", c.cfg.Provider)
}

func (c *cloudStub) UploadReader(_ context.Context, reader io.Reader, size int64, folder string, filename string) (string, string, error) {
	_ = reader
	_ = size
	_ = folder
	_ = filename
	return "", "", fmt.Errorf("cloud provider %s SDK not wired yet; fall back to local storage", c.cfg.Provider)
}

func (c *cloudStub) Download(_ context.Context, storagePath string) (io.ReadCloser, error) {
	_ = storagePath
	return nil, fmt.Errorf("cloud provider %s SDK not wired yet", c.cfg.Provider)
}

func (c *cloudStub) Delete(_ context.Context, storagePath string) error {
	_ = storagePath
	return fmt.Errorf("cloud provider %s SDK not wired yet", c.cfg.Provider)
}

func (c *cloudStub) PublicURL(storagePath string) string {
	return obsConfigAccessURL(c.cfg, storagePath)
}

func (c *cloudStub) SignUploadURL(_ context.Context, _, _, _ string, _ time.Duration) (string, string, error) {
	return "", "", fmt.Errorf("cloud provider %s SDK not wired yet", c.cfg.Provider)
}

func (c *cloudStub) Exists(_ context.Context, storagePath string) (bool, error) {
	_ = storagePath
	return false, fmt.Errorf("cloud provider %s SDK not wired yet", c.cfg.Provider)
}

func (c *cloudStub) Type() string { return string(c.cfg.Provider) }

func obsConfigAccessURL(c *model.ObsConfig, filePath string) string {
	if c.Domain != "" {
		return c.Domain + "/" + filePath
	}
	switch c.Provider {
	case model.ObsProviderAliyun:
		return "https://" + c.Bucket + "." + c.Region + ".aliyuncs.com/" + filePath
	case model.ObsProviderQiniu:
		return "http://" + c.Domain + "/" + filePath
	case model.ObsProviderTencent:
		return "https://" + c.Bucket + ".cos." + c.Region + ".myqcloud.com/" + filePath
	case model.ObsProviderAWS:
		return "https://" + c.Bucket + ".s3." + c.Region + ".amazonaws.com/" + filePath
	default:
		return filePath
	}
}
