package config

import (
	"os"

	"gopkg.in/yaml.v3"
	"marketing/internal/pkg/utils/logger"
)

// DBType 数据库类型（本系统统一使用 PostgreSQL）
type DBType string

const (
	// DBTypePostgres PostgreSQL 数据库类型（唯一支持的关系型数据库）
	DBTypePostgres DBType = "postgres"
)

// VectorDBType 向量数据库类型
type VectorDBType string

const (
	// VectorDBTypePGVector pgvector 向量数据库类型（唯一支持的向量数据库）
	VectorDBTypePGVector VectorDBType = "pgvector"
)

type PlatformCfg struct {
	PlatformAPI struct {
		BaseURL string `yaml:"base_url"`
	} `yaml:"platform_api"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Type     DBType         `yaml:"type"`
	Postgres PostgresConfig `yaml:"postgres"`
	Pool     PoolConfig     `yaml:"pool"`
}

// PoolConfig 数据库连接池配置
type PoolConfig struct {
	MaxIdleConns    int `yaml:"max_idle_conns"`     // 最大空闲连接数
	MaxOpenConns    int `yaml:"max_open_conns"`     // 最大打开连接数
	ConnMaxIdleTime int `yaml:"conn_max_idle_time"` // 连接最大空闲时间（秒）
	ConnMaxLifetime int `yaml:"conn_max_lifetime"`  // 连接最大生存时间（秒）
}

// DefaultPoolConfig 默认连接池配置
//
// 面向 2000 万/日（主动 1000 万 + 被动 1000 万）单节点工作站。
// AI 生成已与接入 worker 解耦，DB 连接仅在短暂的读写期间占用、不跨越数秒级 LLM 推理，
// 因此可将连接池放大以吸收峰值并发；同时设 ConnMaxLifetime 回收长生命周期连接避免陈旧连接。
var DefaultPoolConfig = PoolConfig{
	MaxIdleConns:    50,
	MaxOpenConns:    200,
	ConnMaxIdleTime: 300,  // 5 分钟
	ConnMaxLifetime: 3600, // 60 分钟
}

// 默认推理栈配置常量（私域部署基线）
//
// 设计目的：当 config.yaml 缺失时（如 Docker 首次启动），由 config 包提供
// 与 dev 档一致的本地推理栈默认值，避免消费方（dispatcher/embedding/rerank）
// 各自做兜底硬编码导致契约分裂。
//
// 原则：
//   - 默认值必须指向本地（127.0.0.1），绝不静默走公网
//   - 维度必须与 pgvector vector(1024) 对齐
//   - 模型名必须与实际部署的模型一致（Qwen2.5-1.5B-Instruct / bge-m3 / bge-reranker-v2-m3）
//
// 端口字面量全部派生自 ports.go（DEVELOPMENT.md §2.4 端口对照表单一源）
const (
	defaultLLMModel = "Qwen2.5-1.5B-Instruct"
	// defaultLLMBaseURL 派生自 config.DefaultLLMBaseURLDev
	defaultLLMBaseURL      = DefaultLLMBaseURLDev
	defaultEmbeddingModel  = "bge-m3"
	defaultEmbeddingBaseURL = DefaultEmbeddingBaseURLDev
	defaultRerankModel     = "bge-reranker-v2-m3"
	defaultRerankBaseURL   = DefaultRerankBaseURLDev
	defaultEmbeddingDim    = 1024
)

// DefaultInferenceConfig 返回本地推理栈默认配置（私域部署基线）
//
// 用于 config.yaml 缺失时的回落，确保消费方拿到的 InferenceConfig 永远非零值。
// 消费方（dispatcher 等）不再需要任何模型名/URL 兜底硬编码。
func DefaultInferenceConfig() InferenceConfig {
	return InferenceConfig{
		Profile: "dev",
		Embedding: InferenceEmbeddingConfig{
			Mode:      InferenceModeLocal,
			BaseURL:   defaultEmbeddingBaseURL,
			Model:     defaultEmbeddingModel,
			Dimension: defaultEmbeddingDim,
			// AllowFallback 默认 false：禁止静默降级哈希伪向量（私域基线）
		},
		Rerank: InferenceRerankConfig{
			Mode:    InferenceModeLocal,
			BaseURL: defaultRerankBaseURL,
			Model:   defaultRerankModel,
			Enabled: true,
		},
		LLM: InferenceLLMConfig{
			Mode:           InferenceModeLocal,
			BaseURL:        defaultLLMBaseURL,
			Model:          defaultLLMModel,
			Temperature:    0.7,
			MaxTokens:      1024,
			TimeoutSeconds: 720,
			MaxRetries:     1,
			// NoFC 默认 false：由 dispatcher 基于 URL 启发式判定（本地 true / 云端 false）
		},
	}
}

// PostgresConfig PostgreSQL 配置
type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
	Timezone string `yaml:"timezone"`
}

// VectorDatabaseConfig 向量数据库配置
type VectorDatabaseConfig struct {
	Type     VectorDBType   `yaml:"type"`
	PGVector PGVectorConfig `yaml:"pgvector"`
}

// InferenceMode 推理能力来源模式
type InferenceMode string

const (
	// InferenceModeLocal 默认：走本地推理服务（mtk-embedding/mtk-rerank/mtk-llm 或宿主 127.0.0.1）
	InferenceModeLocal InferenceMode = "local"
	// InferenceModeRemote 走线上 OpenAI 兼容服务（用户配置 base_url+api_key 即生效）
	InferenceModeRemote InferenceMode = "remote"
)

// InferenceConfig 本地推理栈（embedding / rerank / llm）统一配置
//
// 设计要点（详见 docs/architecture/HOST_INFERENCE_PLAN.md）：
//   - 三个能力统一暴露为 OpenAI 兼容 HTTP（/v1/embeddings、/v1/rerank、/v1/chat/completions）。
//   - 默认走本地（mode: local）；用户只需在 base_url+api_key 填线上地址即切到线上（mode 自动 remote）。
//   - profile=dev 选最轻量模型；profile=prod 选效果/硬件平衡；但 embedding 维度必须 1024。
//   - 三级回落：配置文件 inference.* > 环境变量 > 内置本地默认。
type InferenceConfig struct {
	// Profile 仅作文档/脚本快捷选择（dev|prod），真实生效以各子段 model 为准。
	Profile   string                   `yaml:"profile" json:"profile"`
	Embedding InferenceEmbeddingConfig `yaml:"embedding" json:"embedding"`
	Rerank    InferenceRerankConfig    `yaml:"rerank" json:"rerank"`
	LLM       InferenceLLMConfig       `yaml:"llm" json:"llm"`
}

// InferenceEmbeddingConfig 文本向量配置
//
// 私域部署基线：Embedding 默认走本地服务（宿主机 127.0.0.1:8208 / docker mtk-embedding:8208），
// 严禁静默走公网 LLM 厂商 API，也严禁静默降级到哈希伪向量。
//   - base_url: OpenAI 兼容 /v1/embeddings 地址
//   - model:    向量模型名；维度必须与 pgvector vector(N) 一致（硬性 1024）
//   - dimension:向量维度（默认 1024，与 bge-m3 一致）
//   - allow_fallback: 是否允许哈希伪向量降级（默认 false，仅单测显式开启）
type InferenceEmbeddingConfig struct {
	Mode          InferenceMode `yaml:"mode" json:"mode"` // local | remote
	BaseURL       string        `yaml:"base_url" json:"base_url"`
	Model         string        `yaml:"model" json:"model"`
	Dimension     int           `yaml:"dimension" json:"dimension"`
	APIKey        string        `yaml:"api_key" json:"api_key"`
	AllowFallback bool          `yaml:"allow_fallback" json:"allow_fallback"`
}

// InferenceRerankConfig 重排配置
//
// 私域部署基线：Rerank 与 Embedding 同走本地推理服务（宿主机 127.0.0.1:8209 / docker mtk-rerank:8209），
// 使用跨编码器，数据不出域。
type InferenceRerankConfig struct {
	Mode    InferenceMode `yaml:"mode" json:"mode"`
	BaseURL string        `yaml:"base_url" json:"base_url"`
	Model   string        `yaml:"model" json:"model"`
	Enabled bool          `yaml:"enabled" json:"enabled"`
	APIKey  string        `yaml:"api_key" json:"api_key"`
}

// InferenceLLMConfig 大语言模型配置
//
// 默认走本地 mtk-llm（llama.cpp，OpenAI 兼容 /v1/chat/completions）。
// 若 mode=remote 或 base_url 指向线上 OpenAI 兼容服务且 api_key 非空，则自动走线上。
// CloudProviders 为可选的云端 fallback（deepseek/qwen/gpt-4o 等），仅在配置 api_key 后启用。
// NoFC: 标记该 LLM 是否支持 OpenAI Function Calling；本地 Qwen2.5-3B-Instruct 不支持，
// 启用 ReAct 适配器（thought/action 文本协议）走工具调用；用户在 config.yaml
// 设置 inference.llm.no_fc=false 可显式关闭（如果模型已升级支持 FC）。
type InferenceLLMConfig struct {
	Mode           InferenceMode                  `yaml:"mode" json:"mode"`
	BaseURL        string                         `yaml:"base_url" json:"base_url"`
	Model          string                         `yaml:"model" json:"model"`
	APIKey         string                         `yaml:"api_key" json:"api_key"`
	Temperature    float64                        `yaml:"temperature" json:"temperature"`
	MaxTokens      int                            `yaml:"max_tokens" json:"max_tokens"`
	TimeoutSeconds int                            `yaml:"timeout_seconds" json:"timeout_seconds"`
	MaxRetries     int                            `yaml:"max_retries" json:"max_retries"`
	NoFC           bool                           `yaml:"no_fc" json:"no_fc"`
	CloudProviders []InferenceCloudProviderConfig `yaml:"cloud_providers" json:"cloud_providers"`
}

// InferenceCloudProviderConfig 可选的云端 fallback 厂商（OpenAI 兼容）
//
// 仅当用户显式配置 api_key 并 enabled=true 时，dispatcher 才将其注册为可选 fallback。
type InferenceCloudProviderConfig struct {
	Name    string `yaml:"name"`
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	APIType string `yaml:"api_type"`
	Model   string `yaml:"model"`
	Enabled bool   `yaml:"enabled"`
}

// PGVectorConfig pgvector 配置
type PGVectorConfig struct {
	Table     string `yaml:"table"`
	Dimension int    `yaml:"dimension"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	Type  string      `yaml:"type"`
	Qiniu QiniuConfig `yaml:"qiniu"`
}

// QiniuConfig 七牛云配置
type QiniuConfig struct {
	AccessKey    string `yaml:"access_key"`
	SecretKey    string `yaml:"secret_key"`
	Bucket       string `yaml:"bucket"`
	Domain       string `yaml:"domain"`
	UploadDomain string `yaml:"upload_domain"`
	Region       string `yaml:"region"`
}

// AppConfig 应用配置
type AppConfig struct {
	Database       DatabaseConfig       `yaml:"database"`
	VectorDatabase VectorDatabaseConfig `yaml:"vector_database"`
	Inference      InferenceConfig      `yaml:"inference"`
	Storage        StorageConfig        `yaml:"storage"`
	Logging        logger.LoggingConfig `yaml:"logging"`
	I18n           I18nConfig           `yaml:"i18n"`
}

// I18nConfig 多语言方案配置（v1.2 出海多语言）
//
// 包含三个子段：
//   - embedding：bge-m3 多语言 embedding provider 配置（独立于 inference.embedding，
//     可为多语言路径指定不同的 base_url/model/api_key）
//   - cache：跨语言回复翻译缓存配置（Redis 后端）
//   - fallback：低资源语言降级桥配置（P1-2：DeepL 翻译降级）
type I18nConfig struct {
	Embedding I18nEmbeddingConfig `yaml:"embedding" json:"embedding"`
	Cache     I18nCacheConfig     `yaml:"cache" json:"cache"`
	Fallback  I18nFallbackConfig  `yaml:"fallback" json:"fallback"`
}

// I18nFallbackConfig 低资源语言降级桥配置（P1-2）。
//
// 启用条件：enabled=true 且 deepl.api_key 非空。
// 未配置 api_key 时 FallbackBridge 自动禁用，不影响主流程。
type I18nFallbackConfig struct {
	Enabled          bool        `yaml:"enabled" json:"enabled"`                       // 总开关，默认 false
	LowResourceLangs []string    `yaml:"low_resource_langs" json:"low_resource_langs"` // 低资源语言列表，空则用默认 [ar,th,vi,hi,tr]
	Translator       string      `yaml:"translator" json:"translator"`                 // 翻译引擎：deepl（默认）/ google / nllb（未来）
	DeepL            DeepLConfig `yaml:"deepl" json:"deepl"`                           // DeepL 翻译引擎配置
}

// DeepLConfig DeepL 翻译服务配置。
type DeepLConfig struct {
	APIKey  string `yaml:"api_key" json:"api_key"`   // DeepL API key（空则禁用；通过 ${DEEPL_API_KEY} 注入）
	BaseURL string `yaml:"base_url" json:"base_url"` // API 地址，空则用 https://api.deepl.com/v2
}

// I18nEmbeddingConfig 多语言 embedding provider 配置
//
// provider 类型：
//   - "openai"（默认）：复用 inference.embedding 配置（基于 llm.EmbeddingService）
//   - "bge-m3"：显式 bge-m3 provider，走 OpenAI 兼容 /v1/embeddings，
//     支持 normalize / batch_size 等 bge-m3 专属参数
//
// 向后兼容：provider 为空时回退 "openai"。
type I18nEmbeddingConfig struct {
	Provider  string `yaml:"provider" json:"provider"`     // openai / bge-m3
	Model     string `yaml:"model" json:"model"`           // BAAI/bge-m3
	BaseURL   string `yaml:"base_url" json:"base_url"`     // OpenAI 兼容 /v1 根路径
	APIKey    string `yaml:"api_key" json:"api_key"`       // 鉴权密钥（本地可空）
	Dimension int    `yaml:"dimension" json:"dimension"`   // 向量维度，默认 1024
	Normalize bool   `yaml:"normalize" json:"normalize"`   // L2 归一化（bge-m3 推荐 true）
	BatchSize int    `yaml:"batch_size" json:"batch_size"` // 单批最大文本数，默认 32
}

// I18nCacheConfig 跨语言翻译缓存配置
type I18nCacheConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`         // 是否启用（默认 false，显式开启）
	TTL        int    `yaml:"ttl" json:"ttl"`                 // TTL（秒），默认 3600（1h）
	KeyPrefix  string `yaml:"key_prefix" json:"key_prefix"`   // Redis key 前缀，默认 "i18n:trans:"
	MaxEntries int    `yaml:"max_entries" json:"max_entries"` // 最大条目数上限（参考）
}

// GetLoggingConfig 返回统一日志配置；缺省段时回落到日志包默认配置。
func GetLoggingConfig() logger.LoggingConfig {
	return GetAppConfig().Logging
}

func GetServerBaseURL() string {
	// 优先读取配置文件 marketing/config.yaml
	data, err := os.ReadFile("config.yaml")
	if err == nil {
		var cfg PlatformCfg
		if yaml.Unmarshal(data, &cfg) == nil && cfg.PlatformAPI.BaseURL != "" {
			return cfg.PlatformAPI.BaseURL
		}
	}
	// 其次读取环境变量
	if v := os.Getenv("SERVER_API_BASE"); v != "" {
		return v
	}
	// 默认地址（端口派生自 config.DefaultPlatformPort，DEVELOPMENT.md §2.4 端口对照表）
	return DefaultPlatformBaseURL
}

// GetAppConfig 获取应用配置
// 本系统仅使用 PostgreSQL，缺少 config.yaml 时使用 Docker 网络默认配置，
// 由 merchant_init 流程在部署阶段确保 config.yaml 已生成。
func GetAppConfig() AppConfig {
	var config AppConfig

	// 尝试从配置文件读取
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		// 配置文件不存在时使用 Docker 网络默认值（私域部署基线）
		config.Database.Type = DBTypePostgres
		config.Database.Postgres.Host = "postgres-user"
		// 端口派生自 config.DefaultDBPortDocker（DEVELOPMENT.md §2.4 端口对照表）
		config.Database.Postgres.Port = DefaultDBPortDocker
		config.Database.Postgres.User = "admin"
		// 私域合规基线 §7.2：密码不落配置文件，缺配置时由运行时环境变量 POSTGRES_PASSWORD 注入
		config.Database.Postgres.Password = os.Getenv("POSTGRES_PASSWORD")
		config.Database.Postgres.DBName = "user_db"
		config.Database.Postgres.SSLMode = "disable"
		config.VectorDatabase.Type = VectorDBTypePGVector
		config.VectorDatabase.PGVector.Table = "knowledge_embeddings"
		config.VectorDatabase.PGVector.Dimension = 1024 // 2026-07-18 私域基线：本地 TEI + bge-m3（1024 维）
		// 本地推理栈缺省值由 DefaultInferenceConfig 统一提供（本地 127.0.0.1，维度 1024，
		// 绝不静默走公网）；消费方不再做兜底硬编码，配置文件优先于默认值。
		config.Inference = DefaultInferenceConfig()
		return config
	}

	// 支持配置值引用环境变量（如 ${QINIU_ACCESS_KEY}），满足合规基线 §7.2 敏感数据脱敏
	err = yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), &config)
	if err != nil {
		panic(err)
	}

	// 强制类型为 PostgreSQL（防止历史配置残留非 PG 类型）
	config.Database.Type = DBTypePostgres

	return config
}
