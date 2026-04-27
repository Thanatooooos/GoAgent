package chat

type StreamCallback interface {
	OnContent(content string)

	OnThinking(content string)

	OnComplete()

	OnError(err error)
}
