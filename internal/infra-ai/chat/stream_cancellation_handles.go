package chat

type noopStreamCancellationHandle struct{}

func (noopStreamCancellationHandle) Cancel() {}

func NoopStreamCancellationHandle() StreamCancellationHandle {
	return noopStreamCancellationHandle{}
}
