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

type ConversationFieldSet struct {
	ID             Field[string]
	ConversationID Field[string]
	UserID         Field[string]
	Title          Field[string]
	LastTime       Field[*time.Time]
	UpdateTime     Field[time.Time]
}

var Conversation = ConversationFieldSet{
	ID:             Field[string]{Key: "conversation.id"},
	ConversationID: Field[string]{Key: "conversation.conversation_id"},
	UserID:         Field[string]{Key: "conversation.user_id"},
	Title:          Field[string]{Key: "conversation.title"},
	LastTime:       Field[*time.Time]{Key: "conversation.last_time"},
	UpdateTime:     Field[time.Time]{Key: "conversation.update_time"},
}

type MessageFeedbackFieldSet struct {
	ID             Field[string]
	MessageID      Field[string]
	ConversationID Field[string]
	UserID         Field[string]
	Vote           Field[int]
	Reason         Field[string]
	Comment        Field[string]
	UpdateTime     Field[time.Time]
}

var MessageFeedback = MessageFeedbackFieldSet{
	ID:             Field[string]{Key: "message_feedback.id"},
	MessageID:      Field[string]{Key: "message_feedback.message_id"},
	ConversationID: Field[string]{Key: "message_feedback.conversation_id"},
	UserID:         Field[string]{Key: "message_feedback.user_id"},
	Vote:           Field[int]{Key: "message_feedback.vote"},
	Reason:         Field[string]{Key: "message_feedback.reason"},
	Comment:        Field[string]{Key: "message_feedback.comment"},
	UpdateTime:     Field[time.Time]{Key: "message_feedback.update_time"},
}

type RagTraceRunFieldSet struct {
	ID             Field[string]
	TraceID        Field[string]
	TraceName      Field[string]
	EntryMethod    Field[string]
	ConversationID Field[string]
	TaskID         Field[string]
	UserID         Field[string]
	Status         Field[string]
	ErrorMessage   Field[string]
	StartTime      Field[*time.Time]
	EndTime        Field[*time.Time]
	DurationMs     Field[*int64]
	ExtraData      Field[string]
	UpdateTime     Field[time.Time]
}

var RagTraceRun = RagTraceRunFieldSet{
	ID:             Field[string]{Key: "rag_trace_run.id"},
	TraceID:        Field[string]{Key: "rag_trace_run.trace_id"},
	TraceName:      Field[string]{Key: "rag_trace_run.trace_name"},
	EntryMethod:    Field[string]{Key: "rag_trace_run.entry_method"},
	ConversationID: Field[string]{Key: "rag_trace_run.conversation_id"},
	TaskID:         Field[string]{Key: "rag_trace_run.task_id"},
	UserID:         Field[string]{Key: "rag_trace_run.user_id"},
	Status:         Field[string]{Key: "rag_trace_run.status"},
	ErrorMessage:   Field[string]{Key: "rag_trace_run.error_message"},
	StartTime:      Field[*time.Time]{Key: "rag_trace_run.start_time"},
	EndTime:        Field[*time.Time]{Key: "rag_trace_run.end_time"},
	DurationMs:     Field[*int64]{Key: "rag_trace_run.duration_ms"},
	ExtraData:      Field[string]{Key: "rag_trace_run.extra_data"},
	UpdateTime:     Field[time.Time]{Key: "rag_trace_run.update_time"},
}

type RagTraceNodeFieldSet struct {
	ID           Field[string]
	TraceID      Field[string]
	NodeID       Field[string]
	ParentNodeID Field[string]
	Depth        Field[int]
	NodeType     Field[string]
	NodeName     Field[string]
	ClassName    Field[string]
	MethodName   Field[string]
	Status       Field[string]
	ErrorMessage Field[string]
	StartTime    Field[*time.Time]
	EndTime      Field[*time.Time]
	DurationMs   Field[*int64]
	ExtraData    Field[string]
	UpdateTime   Field[time.Time]
}

var RagTraceNode = RagTraceNodeFieldSet{
	ID:           Field[string]{Key: "rag_trace_node.id"},
	TraceID:      Field[string]{Key: "rag_trace_node.trace_id"},
	NodeID:       Field[string]{Key: "rag_trace_node.node_id"},
	ParentNodeID: Field[string]{Key: "rag_trace_node.parent_node_id"},
	Depth:        Field[int]{Key: "rag_trace_node.depth"},
	NodeType:     Field[string]{Key: "rag_trace_node.node_type"},
	NodeName:     Field[string]{Key: "rag_trace_node.node_name"},
	ClassName:    Field[string]{Key: "rag_trace_node.class_name"},
	MethodName:   Field[string]{Key: "rag_trace_node.method_name"},
	Status:       Field[string]{Key: "rag_trace_node.status"},
	ErrorMessage: Field[string]{Key: "rag_trace_node.error_message"},
	StartTime:    Field[*time.Time]{Key: "rag_trace_node.start_time"},
	EndTime:      Field[*time.Time]{Key: "rag_trace_node.end_time"},
	DurationMs:   Field[*int64]{Key: "rag_trace_node.duration_ms"},
	ExtraData:    Field[string]{Key: "rag_trace_node.extra_data"},
	UpdateTime:   Field[time.Time]{Key: "rag_trace_node.update_time"},
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

type ConversationConditions struct {
	ID             string
	ConversationID string
	UserID         string
}

type ConversationPatch struct {
	ConversationID UpdateValue[string]
	UserID         UpdateValue[string]
	Title          UpdateValue[string]
	LastTime       UpdateValue[*time.Time]
	UpdateTime     UpdateValue[time.Time]
}

type MessageFeedbackConditions struct {
	ID             string
	MessageID      string
	ConversationID string
	UserID         string
}

type MessageFeedbackPatch struct {
	MessageID      UpdateValue[string]
	ConversationID UpdateValue[string]
	UserID         UpdateValue[string]
	Vote           UpdateValue[int]
	Reason         UpdateValue[string]
	Comment        UpdateValue[string]
	UpdateTime     UpdateValue[time.Time]
}

type RagTraceRunConditions struct {
	ID             string
	TraceID        string
	ConversationID string
	TaskID         string
	UserID         string
	StatusEQ       string
	StatusNE       string
}

type RagTraceRunPatch struct {
	TraceName      UpdateValue[string]
	EntryMethod    UpdateValue[string]
	ConversationID UpdateValue[string]
	TaskID         UpdateValue[string]
	UserID         UpdateValue[string]
	Status         UpdateValue[string]
	ErrorMessage   UpdateValue[string]
	StartTime      UpdateValue[*time.Time]
	EndTime        UpdateValue[*time.Time]
	DurationMs     UpdateValue[*int64]
	ExtraData      UpdateValue[string]
	UpdateTime     UpdateValue[time.Time]
}

type RagTraceNodeConditions struct {
	ID           string
	TraceID      string
	NodeID       string
	ParentNodeID string
	StatusEQ     string
	StatusNE     string
}

type RagTraceNodePatch struct {
	ParentNodeID UpdateValue[string]
	Depth        UpdateValue[int]
	NodeType     UpdateValue[string]
	NodeName     UpdateValue[string]
	ClassName    UpdateValue[string]
	MethodName   UpdateValue[string]
	Status       UpdateValue[string]
	ErrorMessage UpdateValue[string]
	StartTime    UpdateValue[*time.Time]
	EndTime      UpdateValue[*time.Time]
	DurationMs   UpdateValue[*int64]
	ExtraData    UpdateValue[string]
	UpdateTime   UpdateValue[time.Time]
}
