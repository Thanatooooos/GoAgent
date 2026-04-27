# OpenAIStyleSseParser 使用说明

## 作用

`OpenAIStyleSseParser` 用于解析 OpenAI 兼容协议的 SSE 流式响应单行内容，行为与 `ragent` 中 `infra-ai/chat/OpenAIStyleSseParser` 保持一致。

它支持以下场景：

- 自动忽略空行
- 识别 `data:` 前缀
- 识别 `data: [DONE]` 结束标记
- 从 `choices[0].delta.content` 提取增量内容
- 从 `choices[0].message.content` 提取完整内容
- 在开启 reasoning 时，从 `choices[0].delta.reasoning_content` 或 `choices[0].message.reasoning_content` 提取思考内容
- 当 `choices[0].finish_reason` 非空时，将事件标记为完成

## 对外 API

位置：`internal/infra-ai/chat/openai_style_sse_parser.go`

可用入口：

- `chat.NewOpenAIStyleSseParser(reasoningEnabled bool)`
- `chat.ParseOpenAIStyleSseLine(line string, reasoningEnabled bool)`

解析结果类型：

```go
type ParsedEvent struct {
    Content   string
    Reasoning string
    Completed bool
}
```

辅助方法：

- `HasContent() bool`
- `HasReasoning() bool`

## 使用示例

```go
parser := chat.NewOpenAIStyleSseParser(true)

for _, line := range lines {
    event, err := parser.ParseLine(line)
    if err != nil {
        callback.OnError(err)
        return
    }

    if event.HasReasoning() {
        callback.OnThinking(event.Reasoning)
    }
    if event.HasContent() {
        callback.OnContent(event.Content)
    }
    if event.Completed {
        callback.OnComplete()
        return
    }
}
```

也可以直接使用函数式入口：

```go
event, err := chat.ParseOpenAIStyleSseLine(line, true)
```

## 输入示例

增量内容：

```text
data: {"choices":[{"delta":{"content":"你好"}}]}
```

思考内容：

```text
data: {"choices":[{"delta":{"reasoning_content":"我先分析一下问题"}}]}
```

完整消息：

```text
data: {"choices":[{"message":{"content":"最终答案"}}]}
```

结束事件：

```text
data: [DONE]
```

## 接入建议

如果后续在 `goagent` 中新增 OpenAI 兼容模型客户端，建议在读取 SSE 行后统一调用该解析器，再把结果转发到 `StreamCallback`：

- `event.Reasoning -> callback.OnThinking`
- `event.Content -> callback.OnContent`
- `event.Completed -> callback.OnComplete`

这样可以把协议解析与具体 HTTP/重试/路由逻辑解耦，后续扩展不同供应商时也更容易复用。
