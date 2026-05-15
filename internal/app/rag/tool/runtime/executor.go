package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/log"
)

// Executor 负责按统一规则执行 registry 中的 tool。
type Executor struct {
	registry    *Registry
	middlewares []ToolMiddleware
}

// NewExecutor 创建 tool executor。
func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

// SetMiddlewares sets the middleware chain applied to every tool call.
// Middleware are applied outermost-first: the first element wraps the second, etc.
func (e *Executor) SetMiddlewares(middlewares ...ToolMiddleware) {
	e.middlewares = append([]ToolMiddleware(nil), middlewares...)
}

// Execute 执行单次 tool 调用，并标准化结果结构。
func (e *Executor) Execute(ctx context.Context, call Call) (Result, error) {
	if e == nil || e.registry == nil {
		return Result{}, fmt.Errorf("tool executor registry is required")
	}
	if err := call.Validate(); err != nil {
		return Result{}, err
	}

	module, ok := e.registry.GetModule(call.Name)
	if !ok {
		return Result{
			Name:         strings.TrimSpace(call.Name),
			Status:       CallStatusFailed,
			ErrorMessage: fmt.Sprintf("tool %q not found", strings.TrimSpace(call.Name)),
		}, fmt.Errorf("tool %q not found", strings.TrimSpace(call.Name))
	}

	startedAt := time.Now()
	log.Infof("[tool] %s started: args=%v", strings.TrimSpace(call.Name), summarizeArgs(call.Arguments))

	invoke := func(ctx context.Context, call Call) (Result, error) {
		return module.Invoker.Invoke(ctx, call)
	}
	handler := ApplyMiddleware(invoke, e.middlewares...)
	result, err := handler(ctx, call)

	elapsed := time.Since(startedAt)
	module = module.Normalize()

	if strings.TrimSpace(result.Name) == "" {
		result.Name = module.Spec.Definition.Name
	}
	if strings.TrimSpace(result.Status) == "" {
		if err != nil {
			result.Status = CallStatusFailed
		} else {
			result.Status = CallStatusSuccess
		}
	}
	if err != nil && strings.TrimSpace(result.ErrorMessage) == "" {
		result.ErrorMessage = err.Error()
	}
	result.Meta = mergeResultMeta(module.Spec.ResultMeta(), result.Meta)

	status := strings.TrimSpace(result.Status)
	switch status {
	case CallStatusSuccess:
		log.Infof("[tool] %s success (%dms): %s", result.Name, elapsed.Milliseconds(), TruncateForLog(result.Summary))
	case CallStatusFailed:
		log.Warnf("[tool] %s failed (%dms): %s", result.Name, elapsed.Milliseconds(), TruncateForLog(result.ErrorMessage))
	default:
		log.Infof("[tool] %s %s (%dms)", result.Name, status, elapsed.Milliseconds())
	}

	return result, err
}

func mergeResultMeta(base ResultMeta, override ResultMeta) ResultMeta {
	base = base.Normalize()
	override = override.Normalize()

	if override.Capability != "" {
		base.Capability = override.Capability
	}
	if len(override.EvidenceSources) > 0 {
		base.EvidenceSources = append([]string(nil), override.EvidenceSources...)
	}
	if override.ExecutionMode != "" {
		base.ExecutionMode = override.ExecutionMode
	}
	if override.RiskLevel != "" {
		base.RiskLevel = override.RiskLevel
	}
	if override.ApprovalRequirement != "" {
		base.ApprovalRequirement = override.ApprovalRequirement
	}
	if override.Family != "" {
		base.Family = override.Family
	}
	if override.ReadOnly {
		base.ReadOnly = true
	}
	if override.Terminal {
		base.Terminal = true
	}
	if override.Retryable {
		base.Retryable = true
	}
	return base.Normalize()
}

func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(args))
	for key, value := range args {
		s := fmt.Sprintf("%v", value)
		parts = append(parts, fmt.Sprintf("%s=%s", key, TruncateForLog(s)))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
