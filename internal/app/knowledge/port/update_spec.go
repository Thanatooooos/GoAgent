package port

import "time"

type FieldKey string

type PredicateOperator string

const (
	OperatorEQ        PredicateOperator = "eq"
	OperatorNE        PredicateOperator = "ne"
	OperatorLT        PredicateOperator = "lt"
	OperatorLTE       PredicateOperator = "lte"
	OperatorGT        PredicateOperator = "gt"
	OperatorGTE       PredicateOperator = "gte"
	OperatorIn        PredicateOperator = "in"
	OperatorIsNull    PredicateOperator = "is_null"
	OperatorIsNotNull PredicateOperator = "is_not_null"
)

type Field[T any] struct {
	Key FieldKey
}

func (f Field[T]) Eq(value T) UpdatePredicate {
	return UpdatePredicate{Field: f.Key, Operator: OperatorEQ, Value: value}
}

func (f Field[T]) Ne(value T) UpdatePredicate {
	return UpdatePredicate{Field: f.Key, Operator: OperatorNE, Value: value}
}

func (f Field[T]) Lt(value T) UpdatePredicate {
	return UpdatePredicate{Field: f.Key, Operator: OperatorLT, Value: value}
}

func (f Field[T]) Lte(value T) UpdatePredicate {
	return UpdatePredicate{Field: f.Key, Operator: OperatorLTE, Value: value}
}

func (f Field[T]) Gt(value T) UpdatePredicate {
	return UpdatePredicate{Field: f.Key, Operator: OperatorGT, Value: value}
}

func (f Field[T]) Gte(value T) UpdatePredicate {
	return UpdatePredicate{Field: f.Key, Operator: OperatorGTE, Value: value}
}

func (f Field[T]) In(values ...T) UpdatePredicate {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return UpdatePredicate{Field: f.Key, Operator: OperatorIn, Values: items}
}

func (f Field[T]) IsNull() UpdatePredicate {
	return UpdatePredicate{Field: f.Key, Operator: OperatorIsNull}
}

func (f Field[T]) IsNotNull() UpdatePredicate {
	return UpdatePredicate{Field: f.Key, Operator: OperatorIsNotNull}
}

func (f Field[T]) To(value T) UpdateAssignment {
	return UpdateAssignment{Field: f.Key, Value: value}
}

type UpdatePredicate struct {
	Field    FieldKey
	Operator PredicateOperator
	Value    any
	Values   []any
}

type UpdateAssignment struct {
	Field FieldKey
	Value any
}

type UpdatePredicates []UpdatePredicate

type UpdateAssignments []UpdateAssignment

func Where(predicates ...UpdatePredicate) UpdatePredicates {
	return predicates
}

func Set(assignments ...UpdateAssignment) UpdateAssignments {
	return assignments
}

type KnowledgeDocumentFieldSet struct {
	ID              Field[string]
	KnowledgeBaseID Field[string]
	Name            Field[string]
	Enabled         Field[bool]
	ChunkCount      Field[int]
	FileURL         Field[string]
	FileType        Field[string]
	FileSize        Field[int64]
	ProcessMode     Field[string]
	Status          Field[string]
	SourceType      Field[string]
	SourceLocation  Field[string]
	ScheduleEnabled Field[*bool]
	ScheduleCron    Field[string]
	ChunkStrategy   Field[string]
	ChunkConfig     Field[[]byte]
	PipelineID      Field[string]
	UpdatedBy       Field[string]
	UpdatedAt       Field[time.Time]
	Deleted         Field[bool]
}

var KnowledgeDocument = KnowledgeDocumentFieldSet{
	ID:              Field[string]{Key: "knowledge_document.id"},
	KnowledgeBaseID: Field[string]{Key: "knowledge_document.knowledge_base_id"},
	Name:            Field[string]{Key: "knowledge_document.name"},
	Enabled:         Field[bool]{Key: "knowledge_document.enabled"},
	ChunkCount:      Field[int]{Key: "knowledge_document.chunk_count"},
	FileURL:         Field[string]{Key: "knowledge_document.file_url"},
	FileType:        Field[string]{Key: "knowledge_document.file_type"},
	FileSize:        Field[int64]{Key: "knowledge_document.file_size"},
	ProcessMode:     Field[string]{Key: "knowledge_document.process_mode"},
	Status:          Field[string]{Key: "knowledge_document.status"},
	SourceType:      Field[string]{Key: "knowledge_document.source_type"},
	SourceLocation:  Field[string]{Key: "knowledge_document.source_location"},
	ScheduleEnabled: Field[*bool]{Key: "knowledge_document.schedule_enabled"},
	ScheduleCron:    Field[string]{Key: "knowledge_document.schedule_cron"},
	ChunkStrategy:   Field[string]{Key: "knowledge_document.chunk_strategy"},
	ChunkConfig:     Field[[]byte]{Key: "knowledge_document.chunk_config"},
	PipelineID:      Field[string]{Key: "knowledge_document.pipeline_id"},
	UpdatedBy:       Field[string]{Key: "knowledge_document.updated_by"},
	UpdatedAt:       Field[time.Time]{Key: "knowledge_document.updated_at"},
	Deleted:         Field[bool]{Key: "knowledge_document.deleted"},
}

type UpdateValue[T any] struct {
	Set   bool
	Value T
}

func ValueOf[T any](value T) UpdateValue[T] {
	return UpdateValue[T]{
		Set:   true,
		Value: value,
	}
}

type KnowledgeBaseConditions struct {
	ID           string
	NameEQ       string
	NameNE       string
	CollectionEQ string
	Deleted      *bool
}

type KnowledgeBasePatch struct {
	Name           UpdateValue[string]
	EmbeddingModel UpdateValue[string]
	CollectionName UpdateValue[string]
	UpdatedBy      UpdateValue[string]
	UpdatedAt      UpdateValue[time.Time]
}

type KnowledgeDocumentConditions struct {
	ID              string
	KnowledgeBaseID string
	StatusEQ        string
	StatusNE        string
	SourceTypeEQ    string
	Enabled         *bool
	Deleted         *bool
}

type KnowledgeDocumentPatch struct {
	Name            UpdateValue[string]
	Enabled         UpdateValue[bool]
	ChunkCount      UpdateValue[int]
	FileURL         UpdateValue[string]
	FileType        UpdateValue[string]
	FileSize        UpdateValue[int64]
	ProcessMode     UpdateValue[string]
	Status          UpdateValue[string]
	SourceType      UpdateValue[string]
	SourceLocation  UpdateValue[string]
	ScheduleEnabled UpdateValue[*bool]
	ScheduleCron    UpdateValue[string]
	ChunkStrategy   UpdateValue[string]
	ChunkConfig     UpdateValue[[]byte]
	PipelineID      UpdateValue[string]
	UpdatedBy       UpdateValue[string]
	UpdatedAt       UpdateValue[time.Time]
}

type KnowledgeDocumentScheduleConditions struct {
	ID           string
	DocumentID   string
	Enabled      *bool
	LastStatusEQ string
	LockOwnerEQ  string
}

type KnowledgeDocumentSchedulePatch struct {
	KnowledgeBaseID UpdateValue[string]
	CronExpr        UpdateValue[string]
	Enabled         UpdateValue[bool]
	NextRunTime     UpdateValue[*time.Time]
	LastRunTime     UpdateValue[*time.Time]
	LastSuccessTime UpdateValue[*time.Time]
	LastStatus      UpdateValue[string]
	LastError       UpdateValue[string]
	LastETag        UpdateValue[string]
	LastModified    UpdateValue[string]
	LastContentHash UpdateValue[string]
	LockOwner       UpdateValue[*string]
	LockUntil       UpdateValue[*time.Time]
	UpdatedAt       UpdateValue[time.Time]
}

type KnowledgeDocumentScheduleExecConditions struct {
	ID         string
	ScheduleID string
	DocumentID string
	StatusEQ   string
	StatusNE   string
}

type KnowledgeDocumentScheduleExecPatch struct {
	Status       UpdateValue[string]
	Message      UpdateValue[string]
	StartTime    UpdateValue[*time.Time]
	EndTime      UpdateValue[*time.Time]
	FileName     UpdateValue[string]
	FileSize     UpdateValue[*int64]
	ContentHash  UpdateValue[string]
	ETag         UpdateValue[string]
	LastModified UpdateValue[string]
	UpdatedAt    UpdateValue[time.Time]
}
