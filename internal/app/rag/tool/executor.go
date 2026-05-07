package tool

import (
	"context"
	"fmt"
	"strings"
)

// Executor 负责按统一规则执行 registry 中的 tool。
type Executor struct {
	registry *Registry
}

// NewExecutor 创建 tool executor。
func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

// Execute 执行单次 tool 调用，并标准化结果结构。
func (e *Executor) Execute(ctx context.Context, call Call) (Result, error) {
	if e == nil || e.registry == nil {
		return Result{}, fmt.Errorf("tool executor registry is required")
	}
	if err := call.Validate(); err != nil {
		return Result{}, err
	}

	tool, ok := e.registry.Get(call.Name)
	if !ok {
		return Result{
			Name:         strings.TrimSpace(call.Name),
			Status:       CallStatusFailed,
			ErrorMessage: fmt.Sprintf("tool %q not found", strings.TrimSpace(call.Name)),
		}, fmt.Errorf("tool %q not found", strings.TrimSpace(call.Name))
	}

	result, err := tool.Invoke(ctx, call)
	if strings.TrimSpace(result.Name) == "" {
		result.Name = tool.Definition().Name
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
	return result, err
}
