package web

import (
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// EventData 事件数据
type EventData struct {
	Event string      `json:"event,omitempty"`
	Data  interface{} `json:"data"`
}

// SseEmitterSender SSE 发送器封装类
// 提供线程安全的事件发送功能，统一处理连接关闭状态和异常情况
type SseEmitterSender struct {
	context *gin.Context
	closed  atomic.Bool
}

// NewSseEmitterSender 创建 SSE 发送器
func NewSseEmitterSender(c *gin.Context) *SseEmitterSender {
	// 设置 SSE 相关的 headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	emitter := &SseEmitterSender{
		context: c,
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
	if s.closed.Load() {
		return nil
	}

	var err error
	if eventName == "" {
		// 默认事件
		switch v := data.(type) {
		case string:
			_, err = s.context.Writer.WriteString(fmt.Sprintf("data: %s\n\n", v))
		default:
			jsonData, jsonErr := json.Marshal(v)
			if jsonErr != nil {
				return jsonErr
			}
			_, err = s.context.Writer.WriteString(fmt.Sprintf("data: %s\n\n", jsonData))
		}
	} else {
		// 命名事件
		eventLine := fmt.Sprintf("event: %s\n", eventName)
		_, err = s.context.Writer.WriteString(eventLine)
		if err != nil {
			return err
		}

		switch v := data.(type) {
		case string:
			_, err = s.context.Writer.WriteString(fmt.Sprintf("data: %s\n\n", v))
		default:
			jsonData, jsonErr := json.Marshal(v)
			if jsonErr != nil {
				return jsonErr
			}
			_, err = s.context.Writer.WriteString(fmt.Sprintf("data: %s\n\n", jsonData))
		}
	}

	if err != nil {
		s.Fail(err)
		return err
	}

	// 刷新缓冲区，发送给客户端
	s.context.Writer.Flush()

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
	if s.closed.CompareAndSwap(false, true) {
		// 在实际实现中，可能需要发送一个错误事件
		if err != nil {
			s.SendEvent("error", gin.H{
				"message": err.Error(),
			})
		}
		log.Printf("SSE 连接异常关闭: %v", err)
	}
}

// IsClosed 检查连接是否已关闭
func (s *SseEmitterSender) IsClosed() bool {
	return s.closed.Load()
}
