package domain

import "time"

const (
	MemoryScopeGlobal = "global"
	MemoryScopeKB     = "kb"

	MemoryTypePreference = "preference"
	MemoryTypeKnowledge  = "knowledge"
	MemoryTypeFeedback   = "feedback"

	MemoryStatusPending          = "pending"
	MemoryStatusActive           = "active"
	MemoryStatusRejected         = "rejected"
	MemoryStatusExpired          = "expired"
	MemoryStatusSuperseded       = "superseded"
	MemoryValueTypeText          = "text"
	MemoryValueTypeEnum          = "enum"
	MemoryValueTypeBoolean       = "boolean"
	MemoryValueTypeJSON          = "json"
	MemoryCategoryResponse       = "response"
	MemoryCategoryWorkflow       = "workflow"
	MemoryCategoryBehavior       = "behavior"
	MemoryCategoryProject        = "project"
	MemoryCategoryGeneral        = "general"
	MemoryCategoryFeedback       = "feedback"
	MemoryExtractionMethodManual = "manual"
	MemoryExtractionMethodRule   = "explicit_rule"
	MemoryExtractionMethodLLM    = "explicit_llm"
)

type MemoryItem struct {
	ID string // 记忆项主键
	UserID string // 记忆所属用户
	ScopeType string // 记忆作用域类型，如 global / kb
	ScopeID string // 作用域标识，KB 级记忆时对应具体知识库
	Namespace string // 命名空间，用于按场景隔离记忆
	MemoryType string // 记忆类型，如 preference / knowledge / feedback
	Category string // 记忆业务分类
	CanonicalKey string // 规范化键，用于治理和冲突判定
	ValueType string // 值类型，如 text / enum / boolean / json
	ValueJSON string // 结构化值的规范存储内容
	DisplayValue string // 展示给上层或 prompt 的简短值
	SourceMessageID string // 这条记忆来源的消息 ID
	Content string // 记忆正文
	Summary string // 记忆摘要
	Confidence float64 // 记忆置信度
	Importance int // 记忆重要性，影响排序和保留
	Status string // 当前状态，如 active / expired / superseded
	LastConfirmedAt *time.Time // 最近一次被确认仍然成立的时间
	LastUsedAt *time.Time // 最近一次在 recall 中被使用的时间
	ExpiresAt *time.Time // 过期时间，到期后可转为 expired
	SupersedesID string // 当前记忆替代的旧记忆 ID
	ExtractionMethod string // 记忆写入方式，如 manual / explicit_rule / explicit_llm
	CreatedBy string // 创建者标识
	UpdatedBy string // 最近更新者标识
	CreateTime time.Time // 创建时间
	UpdateTime time.Time // 更新时间
}
