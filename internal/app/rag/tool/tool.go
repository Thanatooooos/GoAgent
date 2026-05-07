package tool

import (
	"context"
	"fmt"
	"strings"
)

const (
	ParamTypeString  = "string"
	ParamTypeNumber  = "number"
	ParamTypeInteger = "integer"
	ParamTypeBoolean = "boolean"
	ParamTypeObject  = "object"
	ParamTypeArray   = "array"
)

// ParameterDefinition 描述单个 tool 参数。
type ParameterDefinition struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// Definition 描述一个 tool 的对外契约。
type Definition struct {
	Name        string
	Description string
	ReadOnly    bool
	Parameters  []ParameterDefinition
}

// Validate 校验 tool 定义是否完整。
func (d Definition) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("tool name is required")
	}
	for _, parameter := range d.Parameters {
		if strings.TrimSpace(parameter.Name) == "" {
			return fmt.Errorf("tool parameter name is required")
		}
	}
	return nil
}

// Call 描述一次 tool 调用请求。
type Call struct {
	Name      string
	Arguments map[string]any
}

// Validate 校验调用请求。
func (c Call) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("tool name is required")
	}
	return nil
}

// Result 描述一次 tool 调用结果。
type Result struct {
	Name         string
	Status       string
	Summary      string
	Data         map[string]any
	ErrorMessage string
}

// Successful 表示 tool 调用是否成功。
func (r Result) Successful() bool {
	return strings.TrimSpace(r.Status) == CallStatusSuccess
}

// Tool 定义单个 tool 的基础接口。
type Tool interface {
	Definition() Definition
	Invoke(ctx context.Context, call Call) (Result, error)
}
