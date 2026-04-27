package chat

import (
	"sync"
	"time"
)

type ProbeResultType int

const (
	ProbeResultSuccess ProbeResultType = iota
	ProbeResultError
	ProbeResultTimeout
	ProbeResultNoContent
)

type ProbeResult struct {
	Type  ProbeResultType
	Error error
}

func (r ProbeResult) IsSuccess() bool {
	return r.Type == ProbeResultSuccess
}

type ProbeStreamBridge struct {
	downstream StreamCallback

	mu        sync.Mutex
	buffer    []func()
	committed bool
	probeCh   chan ProbeResult
	probed    bool
}

func NewProbeStreamBridge(downstream StreamCallback) *ProbeStreamBridge {
	return &ProbeStreamBridge{
		downstream: downstream,
		buffer:     make([]func(), 0, 4),
		probeCh:    make(chan ProbeResult, 1),
	}
}

func (b *ProbeStreamBridge) OnContent(content string) {
	b.completeProbe(ProbeResult{Type: ProbeResultSuccess})
	b.bufferOrDispatch(func() {
		b.downstream.OnContent(content)
	})
}

func (b *ProbeStreamBridge) OnThinking(content string) {
	b.completeProbe(ProbeResult{Type: ProbeResultSuccess})
	b.bufferOrDispatch(func() {
		b.downstream.OnThinking(content)
	})
}

func (b *ProbeStreamBridge) OnComplete() {
	b.completeProbe(ProbeResult{Type: ProbeResultNoContent})
	b.bufferOrDispatch(func() {
		b.downstream.OnComplete()
	})
}

func (b *ProbeStreamBridge) OnError(err error) {
	b.completeProbe(ProbeResult{Type: ProbeResultError, Error: err})
	b.bufferOrDispatch(func() {
		b.downstream.OnError(err)
	})
}

func (b *ProbeStreamBridge) AwaitFirstPacket(timeout time.Duration) ProbeResult {
	if timeout <= 0 {
		timeout = time.Minute
	}

	select {
	case result := <-b.probeCh:
		if result.IsSuccess() {
			b.commit()
		}
		return result
	case <-time.After(timeout):
		return ProbeResult{Type: ProbeResultTimeout}
	}
}

func (b *ProbeStreamBridge) completeProbe(result ProbeResult) {
	b.mu.Lock()
	if b.probed {
		b.mu.Unlock()
		return
	}
	b.probed = true
	ch := b.probeCh
	b.mu.Unlock()

	ch <- result
}

func (b *ProbeStreamBridge) commit() {
	b.mu.Lock()
	if b.committed {
		b.mu.Unlock()
		return
	}
	b.committed = true
	buffer := append([]func(){}, b.buffer...)
	b.buffer = nil
	b.mu.Unlock()

	for _, action := range buffer {
		action()
	}
}

func (b *ProbeStreamBridge) bufferOrDispatch(action func()) {
	b.mu.Lock()
	if !b.committed {
		b.buffer = append(b.buffer, action)
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	action()
}
