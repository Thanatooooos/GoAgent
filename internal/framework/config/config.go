package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 是整个应用的配置根结构，按 ragent 的 application.yaml 建模为强类型结构
type ServerConfig struct {
	Port    int           `mapstructure:"port"`
	Servlet ServletConfig `mapstructure:"servlet"`
}

type ServletConfig struct {
	ContextPath string                 `mapstructure:"context-path"`
	Multipart   ServletMultipartConfig `mapstructure:"multipart"`
}

type ServletMultipartConfig struct {
	MaxFileSize    string `mapstructure:"max-file-size"`
	MaxRequestSize string `mapstructure:"max-request-size"`
}

type SpringConfig struct {
	Application SpringApplicationConfig `mapstructure:"application"`
	Servlet     ServletConfig           `mapstructure:"servlet"`
	Datasource  DataSourceConfig        `mapstructure:"datasource"`
	Data        SpringDataConfig        `mapstructure:"data"`
}

type SpringApplicationConfig struct {
	Name string `mapstructure:"name"`
}

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Spring   SpringConfig   `mapstructure:"spring"`
	RocketMQ RocketMQConfig `mapstructure:"rocketmq"`
	Milvus   MilvusConfig   `mapstructure:"milvus"`
	Rag      RagConfig      `mapstructure:"rag"`
	AI       AIConfig       `mapstructure:"ai"`
	Parser   ParserConfig   `mapstructure:"parser"`
	RustFS   RustFSConfig   `mapstructure:"rustfs"`
	Feishu   FeishuConfig   `mapstructure:"feishu"`
	SaToken  SaTokenConfig  `mapstructure:"sa-token"`
	App      AppConfig      `mapstructure:"app"`
}

// FeishuConfig 飞书开放平台配置。
type FeishuConfig struct {
	AppID     string `mapstructure:"app-id"`
	AppSecret string `mapstructure:"app-secret"`
}

// AICandidate 统一的候选模型配置结构，允许字段冗余以适配 chat/embedding/rerank 场景
type AICandidate struct {
	Id               string `mapstructure:"id"`
	Provider         string `mapstructure:"provider"`
	Model            string `mapstructure:"model"`
	Priority         int    `mapstructure:"priority"`
	SupportsThinking bool   `mapstructure:"supports-thinking"`
	Dimension        int    `mapstructure:"dimension"` // embedding 专用，可为 0
}

type AIChatConfig struct {
	DefaultModel      string        `mapstructure:"default-model"`
	DeepThinkingModel string        `mapstructure:"deep-thinking-model"`
	Candidates        []AICandidate `mapstructure:"candidates"`
}

type AIEmbeddingConfig struct {
	DefaultModel string        `mapstructure:"default-model"`
	Candidates   []AICandidate `mapstructure:"candidates"`
}

type AIRerankConfig struct {
	DefaultModel string        `mapstructure:"default-model"`
	Candidates   []AICandidate `mapstructure:"candidates"`
}
type DataSourceConfig struct {
	DriverClassName string       `mapstructure:"driver-class-name"`
	Type            string       `mapstructure:"type"`
	Username        string       `mapstructure:"username"`
	Password        string       `mapstructure:"password"`
	Url             string       `mapstructure:"url"`
	Hikari          HikariConfig `mapstructure:"hikari"`
}

type HikariConfig struct {
	ConnectionTimeout int    `mapstructure:"connection-timeout"`
	IdleTimeout       int    `mapstructure:"idle-timeout"`
	MaxLifetime       int    `mapstructure:"max-lifetime"`
	MaximumPoolSize   int    `mapstructure:"maximum-pool-size"`
	MinimumIdle       int    `mapstructure:"minimum-idle"`
	PoolName          string `mapstructure:"pool-name"`
}

type SpringDataConfig struct {
	Redis RedisConfig `mapstructure:"redis"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
}

type RocketMQConfig struct {
	NameServer string         `mapstructure:"name-server"`
	Producer   RocketProducer `mapstructure:"producer"`
	Consumer   RocketConsumer `mapstructure:"consumer"`
	Topics     RocketTopics   `mapstructure:"topics"`
}

type RocketProducer struct {
	Group              string `mapstructure:"group"`
	SendMessageTimeout int    `mapstructure:"send-message-timeout"`
}

type RocketConsumer struct {
	ChunkDocumentGroup string `mapstructure:"chunk-document-group"`
}

type RocketTopics struct {
	ChunkDocument         string `mapstructure:"chunk-document"`
	RefreshRemoteDocument string `mapstructure:"refresh-remote-document"`
}

type MilvusConfig struct {
	Uri string `mapstructure:"uri"`
}

type RagConfig struct {
	Vector       RagVectorConfig       `mapstructure:"vector"`
	Default      RagDefaultConfig      `mapstructure:"default"`
	Agent        RagAgentConfig        `mapstructure:"agent"`
	QueryRewrite RagQueryRewriteConfig `mapstructure:"query-rewrite"`
	RateLimit    RagRateLimitConfig    `mapstructure:"rate-limit"`
	Memory       RagMemoryConfig       `mapstructure:"memory"`
	Semaphore    RagSemaphoreConfig    `mapstructure:"semaphore"`
	Knowledge    RagKnowledgeConfig    `mapstructure:"knowledge"`
	Mcp          RagMcpConfig          `mapstructure:"mcp"`
	Search       RagSearchConfig       `mapstructure:"search"`
	Trace        RagTraceConfig        `mapstructure:"trace"`
}

type RagVectorConfig struct {
	Type string `mapstructure:"type"`
}

type RagDefaultConfig struct {
	CollectionName string `mapstructure:"collection-name"`
	Dimension      int    `mapstructure:"dimension"`
	MetricType     string `mapstructure:"metric-type"`
	SseTimeoutMs   int    `mapstructure:"sse-timeout-ms"`
}

type RagAgentConfig struct {
	MaxIterations     int                            `mapstructure:"max-iterations"`
	ParallelToolCalls RagAgentParallelToolCallConfig `mapstructure:"parallel-tool-calls"`
}

type RagAgentParallelToolCallConfig struct {
	Enabled        bool `mapstructure:"enabled"`
	MaxConcurrency int  `mapstructure:"max-concurrency"`
}

type RagQueryRewriteConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	MaxHistoryMessages int  `mapstructure:"max-history-messages"`
	MaxHistoryChars    int  `mapstructure:"max-history-chars"`
}

type RagRateLimitConfig struct {
	Global RagRateLimitGlobal `mapstructure:"global"`
}

type RagRateLimitGlobal struct {
	Enabled        bool `mapstructure:"enabled"`
	MaxConcurrent  int  `mapstructure:"max-concurrent"`
	MaxWaitSeconds int  `mapstructure:"max-wait-seconds"`
	LeaseSeconds   int  `mapstructure:"lease-seconds"`
	PollIntervalMs int  `mapstructure:"poll-interval-ms"`
}

type RagMemoryConfig struct {
	HistoryKeepTurns  int  `mapstructure:"history-keep-turns"`
	SummaryStartTurns int  `mapstructure:"summary-start-turns"`
	SummaryEnabled    bool `mapstructure:"summary-enabled"`
	TtlMinutes        int  `mapstructure:"ttl-minutes"`
	SummaryMaxChars   int  `mapstructure:"summary-max-chars"`
	TitleMaxLength    int  `mapstructure:"title-max-length"`
}

type RagSemaphoreConfig struct {
	DocumentUpload RagSemaphoreItem `mapstructure:"document-upload"`
}

type RagSemaphoreItem struct {
	Name           string `mapstructure:"name"`
	MaxConcurrent  int    `mapstructure:"max-concurrent"`
	MaxWaitSeconds int    `mapstructure:"max-wait-seconds"`
	LeaseSeconds   int    `mapstructure:"lease-seconds"`
}

type RagKnowledgeConfig struct {
	Schedule  RagKnowledgeSchedule  `mapstructure:"schedule"`
	Ingestion RagKnowledgeIngestion `mapstructure:"ingestion"`
}

type RagKnowledgeSchedule struct {
	ScanDelayMs        int `mapstructure:"scan-delay-ms"`
	LockSeconds        int `mapstructure:"lock-seconds"`
	BatchSize          int `mapstructure:"batch-size"`
	MinIntervalSeconds int `mapstructure:"min-interval-seconds"`
}

type RagKnowledgeIngestion struct {
	MaxConcurrent  int `mapstructure:"max-concurrent"`
	MaxRetries     int `mapstructure:"max-retries"`
	RetryBackoffMs int `mapstructure:"retry-backoff-ms"`
}

type RagMcpConfig struct {
	Servers []McpServer `mapstructure:"servers"`
}

type McpServer struct {
	Name string `mapstructure:"name"`
	Url  string `mapstructure:"url"`
}

type RagSearchConfig struct {
	Channels RagSearchChannels `mapstructure:"channels"`
}

type RagSearchChannels struct {
	VectorGlobal   RagSearchChannel `mapstructure:"vector-global"`
	IntentDirected RagSearchChannel `mapstructure:"intent-directed"`
}

type RagSearchChannel struct {
	ConfidenceThreshold float64 `mapstructure:"confidence-threshold"`
	TopKMultiplier      int     `mapstructure:"top-k-multiplier"`
	MinIntentScore      float64 `mapstructure:"min-intent-score"`
}

type RagTraceConfig struct {
	Enabled        bool `mapstructure:"enabled"`
	MaxErrorLength int  `mapstructure:"max-error-length"`
}

// AI 配置（与 Java 的 AIModelProperties 对齐）
type AIConfig struct {
	Providers map[string]ProviderConfig `mapstructure:"providers"`
	Chat      ModelGroup                `mapstructure:"chat"`
	Embedding ModelGroup                `mapstructure:"embedding"`
	Rerank    ModelGroup                `mapstructure:"rerank"`
	Selection Selection                 `mapstructure:"selection"`
	Stream    Stream                    `mapstructure:"stream"`
}

type ProviderConfig struct {
	Url       string            `mapstructure:"url"`
	ApiKey    string            `mapstructure:"api-key"`
	Endpoints map[string]string `mapstructure:"endpoints"`
}

type ModelGroup struct {
	DefaultModel      string           `mapstructure:"default-model"`
	DeepThinkingModel string           `mapstructure:"deep-thinking-model"`
	Candidates        []ModelCandidate `mapstructure:"candidates"`
}

type ModelCandidate struct {
	Id               string `mapstructure:"id"`
	Provider         string `mapstructure:"provider"`
	Model            string `mapstructure:"model"`
	Url              string `mapstructure:"url"`
	Dimension        any    `mapstructure:"dimension"`
	Priority         int    `mapstructure:"priority"`
	Enabled          *bool  `mapstructure:"enabled"`
	SupportsThinking *bool  `mapstructure:"supports-thinking"`
}

// DimensionInt 将可能为 string/int/float 的 Dimension 解析为 int，解析失败时返回默认值 def
func (mc ModelCandidate) DimensionInt(def int) int {
	if mc.Dimension == nil {
		return def
	}
	switch v := mc.Dimension.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	case string:
		// 尝试解析数字字符串，忽略模板或变量形式
		s := strings.TrimSpace(v)
		if s == "" {
			return def
		}
		// 如果包含非数字字符，返回默认值
		var val int
		_, err := fmt.Sscanf(s, "%d", &val)
		if err == nil {
			return val
		}
		return def
	default:
		return def
	}
}

type Selection struct {
	FailureThreshold int   `mapstructure:"failure-threshold"`
	OpenDurationMs   int64 `mapstructure:"open-duration-ms"`
}

type Stream struct {
	MessageChunkSize int `mapstructure:"message-chunk-size"`
}

type ParserConfig struct {
	Tika ParserTikaConfig `mapstructure:"tika"`
}

type ParserTikaConfig struct {
	URL       string `mapstructure:"url"`
	TimeoutMs int    `mapstructure:"timeout-ms"`
}

type RustFSConfig struct {
	Url             string `mapstructure:"url"`
	AccessKeyId     string `mapstructure:"access-key-id"`
	SecretAccessKey string `mapstructure:"secret-access-key"`
	Bucket          string `mapstructure:"bucket"`
}

type SaTokenConfig struct {
	TokenName    string `mapstructure:"token-name"`
	Timeout      int    `mapstructure:"timeout"`
	IsConcurrent bool   `mapstructure:"is-concurrent"`
	IsShare      bool   `mapstructure:"is-share"`
	TokenStyle   string `mapstructure:"token-style"`
	IsLog        bool   `mapstructure:"is-log"`
	IsPrint      bool   `mapstructure:"is-print"`
}

type AppConfig struct {
	DemoMode bool `mapstructure:"demo-mode"`
}

var cfg *Config

// LoadConfig 从指定目录加载 configs/application.(yaml|yml|json)（默认 ./configs）
func LoadConfig(dir string) error {
	if dir == "" {
		dir = "configs"
	}
	v := viper.New()
	v.AddConfigPath(dir)
	v.SetConfigName("application")
	v.SetConfigType("yaml")

	// 支持环境变量替换，使用下划线形式
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}
	cfg = &c
	return nil
}

// Get 返回加载后的全局配置
func Get() *Config {
	return cfg
}
