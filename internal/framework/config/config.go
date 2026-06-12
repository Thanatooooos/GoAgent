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
	Servlet    ServletConfig    `mapstructure:"servlet"`
	Datasource DataSourceConfig `mapstructure:"datasource"`
	Data       SpringDataConfig `mapstructure:"data"`
}

type SpringDataConfig struct {
	Redis RedisConfig `mapstructure:"redis"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Spring  SpringConfig  `mapstructure:"spring"`
	Rag     RagConfig     `mapstructure:"rag"`
	AI      AIConfig      `mapstructure:"ai"`
	Parser  ParserConfig  `mapstructure:"parser"`
	RustFS  RustFSConfig  `mapstructure:"rustfs"`
	Feishu  FeishuConfig  `mapstructure:"feishu"`
	SaToken SaTokenConfig `mapstructure:"sa-token"`
	App     AppConfig     `mapstructure:"app"`
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

type RagConfig struct {
	Vector       RagVectorConfig       `mapstructure:"vector"`
	Default      RagDefaultConfig      `mapstructure:"default"`
	Agent        RagAgentConfig        `mapstructure:"agent"`
	Retrieve     RagRetrieveConfig     `mapstructure:"retrieve"`
	QueryRewrite RagQueryRewriteConfig `mapstructure:"query-rewrite"`
	RateLimit    RagRateLimitConfig    `mapstructure:"rate-limit"`
	Memory       RagMemoryConfig       `mapstructure:"memory"`
	Knowledge    RagKnowledgeConfig    `mapstructure:"knowledge"`
	MCP          RagMCPConfig          `mapstructure:"mcp"`
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
	MaxIterations      int                              `mapstructure:"max-iterations"`
	ParallelToolCalls  RagAgentParallelToolCallConfig   `mapstructure:"parallel-tool-calls"`
	Chat               RagAgentChatConfig               `mapstructure:"chat"`
	RuntimePersistence RagAgentRuntimePersistenceConfig `mapstructure:"runtime-persistence"`
}

type RagAgentParallelToolCallConfig struct {
	Enabled        bool `mapstructure:"enabled"`
	MaxConcurrency int  `mapstructure:"max-concurrency"`
}

type RagAgentChatConfig struct {
	Mode string `mapstructure:"mode"`
}

type RagAgentRuntimePersistenceConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Dir     string `mapstructure:"dir"`
}

type RagRetrieveConfig struct {
	ParallelSubquestions RagRetrieveParallelSubquestionConfig `mapstructure:"parallel-subquestions"`
}

type RagRetrieveParallelSubquestionConfig struct {
	Enabled        bool `mapstructure:"enabled"`
	MaxConcurrency int  `mapstructure:"max-concurrency"`
}

type RagQueryRewriteConfig struct {
	Enabled            bool                       `mapstructure:"enabled"`
	MaxHistoryMessages int                        `mapstructure:"max-history-messages"`
	MaxHistoryChars    int                        `mapstructure:"max-history-chars"`
	TermNormalization  RagTermNormalizationConfig `mapstructure:"term-normalization"`
}

type RagTermNormalizationConfig struct {
	Enabled bool                       `mapstructure:"enabled"`
	Rules   []RagTermNormalizationRule `mapstructure:"rules"`
}

type RagTermNormalizationRule struct {
	Canonical string   `mapstructure:"canonical"`
	Aliases   []string `mapstructure:"aliases"`
	Category  string   `mapstructure:"category"`
	Version   int      `mapstructure:"version"`
	Enabled   *bool    `mapstructure:"enabled"`
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
	HistoryKeepTurns  int                        `mapstructure:"history-keep-turns"`
	SummaryStartTurns int                        `mapstructure:"summary-start-turns"`
	SummaryEnabled    bool                       `mapstructure:"summary-enabled"`
	SummaryAsync      RagSummaryAsyncConfig      `mapstructure:"summary-async"`
	SummaryMaxChars   int                        `mapstructure:"summary-max-chars"`
	TitleMaxLength    int                        `mapstructure:"title-max-length"`
	LongMessage       RagLongMessageConfig       `mapstructure:"long-message"`
	SessionRecall     RagSessionRecallConfig     `mapstructure:"session-recall"`
	ExplicitRecall    RagExplicitRecallConfig    `mapstructure:"explicit-recall"`
	Cache             RagMemoryCacheConfig       `mapstructure:"cache"`
	Maintenance       RagMemoryMaintenanceConfig `mapstructure:"maintenance"`
	ChatContext       RagChatContextConfig       `mapstructure:"chat-context"`
}

type RagChatContextConfig struct {
	Enabled         bool `mapstructure:"enabled"`
	MaxPromptTokens int  `mapstructure:"max-prompt-tokens"`
}

type RagSummaryAsyncConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type RagMemoryCacheConfig struct {
	Enabled                  bool   `mapstructure:"enabled"`
	RequestScopeEnabled      bool   `mapstructure:"request-scope-enabled"`
	ConversationScopeEnabled bool   `mapstructure:"conversation-scope-enabled"`
	SessionRecallEnabled     bool   `mapstructure:"session-recall-enabled"`
	RequestMaxEntries        int    `mapstructure:"request-max-entries"`
	ConversationMaxEntries   int    `mapstructure:"conversation-max-entries"`
	ConversationTTLSeconds   int    `mapstructure:"conversation-ttl-seconds"`
	EmptySessionTTLSeconds   int    `mapstructure:"empty-session-ttl-seconds"`
	MetricsEnabled           bool   `mapstructure:"metrics-enabled"`
	RedisKeyPrefix           string `mapstructure:"redis-key-prefix"`
	EmbeddingTTLSeconds      int    `mapstructure:"embedding-ttl-seconds"`
	RuleTTLSeconds           int    `mapstructure:"rule-ttl-seconds"`
	FactTTLSeconds           int    `mapstructure:"fact-ttl-seconds"`
	EmptyFactTTLSeconds      int    `mapstructure:"empty-fact-ttl-seconds"`
}

type RagMemoryMaintenanceConfig struct {
	Enabled             bool `mapstructure:"enabled"`
	ScanDelayMs         int  `mapstructure:"scan-delay-ms"`
	RunTimeoutMs        int  `mapstructure:"run-timeout-ms"`
	ExpireBatchSize     int  `mapstructure:"expire-batch-size"`
	DeleteBatchSize     int  `mapstructure:"delete-batch-size"`
	DeleteRetentionDays int  `mapstructure:"delete-retention-days"`
}

type RagLongMessageConfig struct {
	Enabled                     bool `mapstructure:"enabled"`
	DirectContextMaxTokens      int  `mapstructure:"direct-context-max-tokens"`
	ChunkSummaryThresholdTokens int  `mapstructure:"chunk-summary-threshold-tokens"`
	LargeChunkTargetTokens      int  `mapstructure:"large-chunk-target-tokens"`
	LargeChunkOverlapTokens     int  `mapstructure:"large-chunk-overlap-tokens"`
	MediumSummaryMaxChars       int  `mapstructure:"medium-summary-max-chars"`
	ChunkSummaryMaxChars        int  `mapstructure:"chunk-summary-max-chars"`
	LargeSummaryMaxChars        int  `mapstructure:"large-summary-max-chars"`
}

type RagSessionRecallConfig struct {
	Enabled              bool `mapstructure:"enabled"`
	MaxExcerpts          int  `mapstructure:"max-excerpts"`
	MaxChunksPerMessage  int  `mapstructure:"max-chunks-per-message"`
	ExcerptTargetTokens  int  `mapstructure:"excerpt-target-tokens"`
	ExcerptOverlapTokens int  `mapstructure:"excerpt-overlap-tokens"`
	MaxPromptTokens      int  `mapstructure:"max-prompt-tokens"`
}

type RagExplicitRecallConfig struct {
	MaxItems              int `mapstructure:"max-items"`
	MaxContextChars       int `mapstructure:"max-context-chars"`
	MaxCandidatesPerScope int `mapstructure:"max-candidates-per-scope"`
}

type RagKnowledgeConfig struct {
	Schedule  RagKnowledgeSchedule  `mapstructure:"schedule"`
	Ingestion RagKnowledgeIngestion `mapstructure:"ingestion"`
}

type RagKnowledgeSchedule struct {
	ScanDelayMs        int `mapstructure:"scan-delay-ms"`
	RunTimeoutMs       int `mapstructure:"run-timeout-ms"`
	LockSeconds        int `mapstructure:"lock-seconds"`
	BatchSize          int `mapstructure:"batch-size"`
	MinIntervalSeconds int `mapstructure:"min-interval-seconds"`
}

type RagKnowledgeIngestion struct {
	MaxConcurrent  int `mapstructure:"max-concurrent"`
	MaxRetries     int `mapstructure:"max-retries"`
	RetryBackoffMs int `mapstructure:"retry-backoff-ms"`
}

type RagSearchConfig struct {
	Channels  RagSearchChannels  `mapstructure:"channels"`
	WebSearch RagWebSearchConfig `mapstructure:"web-search"`
}

type RagMCPConfig struct {
	Servers map[string]RagMCPServerConfig `mapstructure:"servers"`
}

type RagMCPServerConfig struct {
	Enabled          bool              `mapstructure:"enabled"`
	Transport        string            `mapstructure:"transport"`
	Command          string            `mapstructure:"command"`
	Args             []string          `mapstructure:"args"`
	Env              map[string]string `mapstructure:"env"`
	StartupTimeoutMs int               `mapstructure:"startup-timeout-ms"`
	CallTimeoutMs    int               `mapstructure:"call-timeout-ms"`
}

type RagWebSearchConfig struct {
	Provider         string                         `mapstructure:"provider"`          // "duckduckgo", "tavily", or "tavily-mcp"
	FallbackProvider string                         `mapstructure:"fallback-provider"` // "", "duckduckgo", or "tavily"
	ApiKey           string                         `mapstructure:"api-key"`           // required for tavily direct API; reused for Tavily MCP env by default
	MCP              RagWebSearchMCPConfig          `mapstructure:"mcp"`
	SourcePolicy     RagWebSearchSourcePolicyConfig `mapstructure:"source-policy"`
}

type RagWebSearchMCPConfig struct {
	Server     string `mapstructure:"server"`
	SearchTool string `mapstructure:"search-tool"`
}

type RagWebSearchSourcePolicyConfig struct {
	AllowDomains  []string `mapstructure:"allow-domains"`
	DenyDomains   []string `mapstructure:"deny-domains"`
	AllowSuffixes []string `mapstructure:"allow-suffixes"`
	DenySuffixes  []string `mapstructure:"deny-suffixes"`
}

type RagSearchChannels struct {
	VectorGlobal   RagSearchChannel                    `mapstructure:"vector-global"`
	IntentDirected RagSearchChannel                    `mapstructure:"intent-directed"`
	Keyword        RagKeywordSearchChannelConfig       `mapstructure:"keyword"`
	MetadataTitle  RagMetadataTitleSearchChannelConfig `mapstructure:"metadata-title"`
}

type RagSearchChannel struct {
	ConfidenceThreshold float64 `mapstructure:"confidence-threshold"`
	TopKMultiplier      int     `mapstructure:"top-k-multiplier"`
	MinIntentScore      float64 `mapstructure:"min-intent-score"`
}

type RagKeywordSearchChannelConfig struct {
	EnabledFallbackTrgm *bool  `mapstructure:"enabled-fallback-trgm"`
	Backend             string `mapstructure:"backend"`
}

const KeywordBackendBM25 = "bm25"
const KeywordBackendTsvector = "tsvector"

type RagMetadataTitleSearchChannelConfig struct {
	EnabledFallbackTrgm  *bool   `mapstructure:"enabled-fallback-trgm"`
	SectionWeight        float64 `mapstructure:"section-weight"`
	DocumentNameWeight   float64 `mapstructure:"document-name-weight"`
	SourceFileNameWeight float64 `mapstructure:"source-file-name-weight"`
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
