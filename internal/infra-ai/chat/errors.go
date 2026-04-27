package chat

const (
	errNoChatModelCandidates  = "no chat model candidates available"
	errStreamNoProvider       = "no chat model candidates available for stream"
	errStreamStartFailed      = "stream chat start failed"
	errStreamTimeout          = "stream first packet timeout"
	errStreamNoContent        = "stream completed without content"
	errStreamAllFailed        = "all chat model stream candidates failed"
	errRoutingServiceNil      = "routing llm service selector is nil"
	errCallbackNil            = "stream callback is nil"
	errChatProviderMissingFmt = "chat provider client missing: provider=%s modelId=%s"
	errChatModelUnavailable   = "chat model unavailable: %s"
)
