package core

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
	Meta         ResultMeta
}

type ResultMeta struct {
	Capability          string
	EvidenceSources     []string
	ExecutionMode       string
	RiskLevel           string
	ApprovalRequirement string
	ReadOnly            bool
	Family              string
	Terminal            bool
	Retryable           bool
}

func (m ResultMeta) Normalize() ResultMeta {
	m.Capability = strings.TrimSpace(strings.ToLower(m.Capability))
	m.EvidenceSources = UniqueTrimmedStrings(m.EvidenceSources)
	m.ExecutionMode = strings.TrimSpace(strings.ToLower(m.ExecutionMode))
	m.RiskLevel = strings.TrimSpace(strings.ToLower(m.RiskLevel))
	m.ApprovalRequirement = strings.TrimSpace(strings.ToLower(m.ApprovalRequirement))
	m.Family = strings.TrimSpace(strings.ToLower(m.Family))
	return m
}

// Successful 表示 tool 调用是否成功。
func (r Result) Successful() bool {
	return strings.TrimSpace(r.Status) == CallStatusSuccess
}

// GetString reads a string value from the result data.
func (r Result) GetString(key string) string {
	if len(r.Data) == 0 {
		return ""
	}
	value, ok := r.Data[key]
	if !ok || value == nil {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

// GetInt reads an integer value from the result data.
func (r Result) GetInt(key string) int {
	if len(r.Data) == 0 {
		return 0
	}
	value, ok := r.Data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

// GetStringSlice reads a []string value from the result data.
func (r Result) GetStringSlice(key string) []string {
	if len(r.Data) == 0 {
		return []string{}
	}
	value, ok := r.Data[key]
	if !ok || value == nil {
		return []string{}
	}
	switch typed := value.(type) {
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := fmt.Sprintf("%v", item)
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	default:
		return []string{}
	}
}

// PreferStringSlice reads a []string from the primary key; falls back to the fallback key when empty.
func (r Result) PreferStringSlice(primary, fallback string) []string {
	items := r.GetStringSlice(primary)
	if len(items) > 0 {
		return items
	}
	return r.GetStringSlice(fallback)
}

// Tool 定义单个 tool 的基础接口。
type Tool interface {
	Definition() Definition
	Invoke(ctx context.Context, call Call) (Result, error)
}
