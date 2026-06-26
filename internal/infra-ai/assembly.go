package infraai

import (
	"net"
	"net/http"
	"time"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/infra-ai/chat"
	"local/rag-project/internal/infra-ai/embedding"
	"local/rag-project/internal/infra-ai/model"
	"local/rag-project/internal/infra-ai/rerank"
)

const (
	defaultHTTPTimeout   = 60 * time.Second
	defaultStreamTimeout = 5 * time.Minute
)

type Runtime struct {
	HTTPClient       *http.Client
	StreamHTTPClient *http.Client

	HealthStore *model.ModelHealthStore
	Selector    *model.ModelSelector
	Executor    *model.ModelRoutingExecutor

	ChatClients      []chat.ChatClient
	EmbeddingClients []embedding.EmbeddingClient
	RerankClients    []rerank.RerankClient

	Chat      chat.LLMService
	Embedding embedding.EmbeddingService
	Rerank    rerank.RerankService
}

type RuntimeOptions struct {
	HTTPClient       *http.Client
	StreamHTTPClient *http.Client
}

func NewRuntime() *Runtime {
	return NewRuntimeWithOptions(RuntimeOptions{})
}

func NewRuntimeWithOptions(opts RuntimeOptions) *Runtime {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = newDefaultHTTPClient(defaultHTTPTimeout)
	}

	streamClient := opts.StreamHTTPClient
	if streamClient == nil {
		streamClient = newDefaultHTTPClient(streamTimeout())
	}

	healthStore := model.NewModelHealthStore()
	selector := model.NewModelSelector(healthStore)
	executor := model.NewModelRoutingExecutor(healthStore)

	chatClients := chat.NewDefaultOpenAIStyleChatClients(
		httpClient,
		chat.WithStreamHTTPClient(streamClient),
	)
	embeddingClients := embedding.NewDefaultOpenAIStyleEmbeddingClients(httpClient)
	rerankClients := rerank.NewDefaultRerankClients(httpClient)

	chatService := chat.NewRoutingLLmService(selector, healthStore, executor, chatClients)
	embeddingService := embedding.NewRoutingEmbeddingService(selector, executor, embeddingClients)
	rerankService := rerank.NewRoutingRerankService(selector, executor, rerankClients)

	return &Runtime{
		HTTPClient:       httpClient,
		StreamHTTPClient: streamClient,
		HealthStore:      healthStore,
		Selector:         selector,
		Executor:         executor,
		ChatClients:      chatClients,
		EmbeddingClients: embeddingClients,
		RerankClients:    rerankClients,
		Chat:             chatService,
		Embedding:        embeddingService,
		Rerank:           rerankService,
	}
}

func newDefaultHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.MaxConnsPerHost = 50
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = timeout
	transport.DialContext = (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func streamTimeout() time.Duration {
	cfg := config.Get()
	if cfg == nil || cfg.Rag.Default.SseTimeoutMs <= 0 {
		return defaultStreamTimeout
	}
	return time.Duration(cfg.Rag.Default.SseTimeoutMs) * time.Millisecond
}
