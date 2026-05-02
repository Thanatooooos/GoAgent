package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	frameworkconfig "local/rag-project/internal/framework/config"
)

// EventData 事件数据
type EventData struct {
	Event string      `json:"event,omitempty"`
	Data  interface{} `json:"data"`
}

const defaultSSEWriteTimeout = 15 * time.Second

// SseEmitterSender SSE 发送器封装类
// 提供线程安全的事件发送功能，统一处理连接关闭状态和异常情况
type SseEmitterSender struct {
	context      *gin.Context
	closed       atomic.Bool
	mu           sync.Mutex
	writeTimeout time.Duration
}

// NewSseEmitterSender 创建 SSE 发送器
func NewSseEmitterSender(c *gin.Context) *SseEmitterSender {
	// 设置 SSE 相关的 headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	emitter := &SseEmitterSender{
		context:      c,
		writeTimeout: resolveSSEWriteTimeout(),
	}

	// 监听客户端断开连接
	notify := c.Request.Context().Done()
	go func() {
		<-notify
		emitter.closed.Store(true)
		log.Println("SSE 客户端断开连接")
	}()

	return emitter
}

// SendEvent 发送 SSE 事件到客户端
// 支持两种发送模式：
// - eventName 为空时，使用默认事件格式发送数据
// - eventName 不为空时，发送带命名的事件
func (s *SseEmitterSender) SendEvent(eventName string, data interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return nil
	}
	err := s.writeEventLocked(eventName, data)
	if err != nil {
		s.closed.Store(true)
		log.Printf("SSE 发送失败: %v", err)
		return err
	}
	return nil
}

// Complete 正常完成并关闭 SSE 连接
// 使用 CAS 操作确保连接只被关闭一次，方法是幂等的
func (s *SseEmitterSender) Complete() {
	if s.closed.CompareAndSwap(false, true) {
		log.Println("SSE 连接正常关闭")
	}
}

// Fail 异常结束并关闭 SSE 连接
// 当发生异常时调用此方法，会关闭 SSE 连接并通知客户端异常信息
func (s *SseEmitterSender) Fail(err error) {
	if err != nil {
		log.Printf("SSE 发送失败: %v", err)
	}
	s.closeWithError(err)
}

// closeWithError 以异常方式关闭连接
// 使用 CAS 操作确保连接只被关闭一次
func (s *SseEmitterSender) closeWithError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return
	}
	if err != nil {
		_ = s.writeEventLocked("error", gin.H{
			"message": err.Error(),
		})
	}
	s.closed.Store(true)
	log.Printf("SSE 连接异常关闭: %v", err)
}

// IsClosed 检查连接是否已关闭
func (s *SseEmitterSender) IsClosed() bool {
	return s.closed.Load()
}

func (s *SseEmitterSender) writeEventLocked(eventName string, data interface{}) error {
	if s == nil || s.context == nil {
		return errors.New("sse emitter is required")
	}
	if err := s.context.Request.Context().Err(); err != nil {
		return err
	}

	payload, err := buildSSEPayload(eventName, data)
	if err != nil {
		return err
	}
	resetWriteDeadline, err := s.applyWriteDeadlineLocked()
	if err != nil {
		return err
	}
	defer resetWriteDeadline()
	if _, err := s.context.Writer.WriteString(payload); err != nil {
		return err
	}

	flusher, ok := s.context.Writer.(http.Flusher)
	if !ok {
		return errors.New("sse writer does not support flush")
	}
	flusher.Flush()
	return nil
}

func (s *SseEmitterSender) applyWriteDeadlineLocked() (func(), error) {
	if s.writeTimeout <= 0 {
		return func() {}, nil
	}
	controller := http.NewResponseController(s.context.Writer)
	if err := controller.SetWriteDeadline(time.Now().Add(s.writeTimeout)); err != nil {
		// 非所有 writer 都支持 deadline，保持降级兼容。
		if !errors.Is(err, http.ErrNotSupported) {
			return nil, err
		}
		return func() {}, nil
	}
	return func() {
		_ = controller.SetWriteDeadline(time.Time{})
	}, nil
}

func buildSSEPayload(eventName string, data interface{}) (string, error) {
	if eventName == "" {
		switch v := data.(type) {
		case string:
			return fmt.Sprintf("data: %s\n\n", v), nil
		default:
			jsonData, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("data: %s\n\n", jsonData), nil
		}
	}

	switch v := data.(type) {
	case string:
		return fmt.Sprintf("event: %s\ndata: %s\n\n", eventName, v), nil
	default:
		jsonData, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("event: %s\ndata: %s\n\n", eventName, jsonData), nil
	}
}

func resolveSSEWriteTimeout() time.Duration {
	cfg := frameworkconfig.Get()
	if cfg != nil && cfg.Rag.Default.SseTimeoutMs > 0 {
		return time.Duration(cfg.Rag.Default.SseTimeoutMs) * time.Millisecond
	}
	return defaultSSEWriteTimeout
}
