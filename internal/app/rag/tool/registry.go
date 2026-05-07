package tool

import (
	"fmt"
	"sort"
	"strings"
)

// Registry 负责 tool 注册与查询。
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 创建空 registry。
func NewRegistry() *Registry {
	return &Registry{
		tools: map[string]Tool{},
	}
}

// Register 注册单个 tool。
func (r *Registry) Register(tool Tool) error {
	if r == nil {
		return fmt.Errorf("tool registry is required")
	}
	if tool == nil {
		return fmt.Errorf("tool is required")
	}

	definition := tool.Definition()
	if err := definition.Validate(); err != nil {
		return err
	}
	name := strings.TrimSpace(definition.Name)
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = tool
	return nil
}

// MustRegister 注册 tool，失败时 panic。
func (r *Registry) MustRegister(tool Tool) {
	if err := r.Register(tool); err != nil {
		panic(err)
	}
}

// Get 查询指定名称的 tool。
func (r *Registry) Get(name string) (Tool, bool) {
	if r == nil {
		return nil, false
	}
	tool, ok := r.tools[strings.TrimSpace(name)]
	return tool, ok
}

// ListDefinitions 返回按名称排序的 tool 定义列表。
func (r *Registry) ListDefinitions() []Definition {
	if r == nil || len(r.tools) == 0 {
		return []Definition{}
	}
	items := make([]Definition, 0, len(r.tools))
	for _, tool := range r.tools {
		items = append(items, tool.Definition())
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}
