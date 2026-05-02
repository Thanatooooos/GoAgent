package rag

import (
	"gorm.io/gorm"

	postgrescommon "local/rag-project/internal/adapter/repository/postgres/common"
	"local/rag-project/internal/app/rag/port"
)

type updateFieldColumnResolver func(port.FieldKey) (string, bool)

type updateFieldValueMapper func(port.FieldKey, any) (any, error)

func conditionalUpdateRequiresConditions(entity string) error {
	return postgrescommon.ConditionalUpdateRequiresConditions(entity)
}

func applyUpdatePredicates(
	query *gorm.DB,
	predicates port.UpdatePredicates,
	columnFor updateFieldColumnResolver,
	valueFor updateFieldValueMapper,
) (*gorm.DB, error) {
	return postgrescommon.ApplyUpdatePredicates(
		query,
		predicates,
		func(item port.UpdatePredicate) port.FieldKey { return item.Field },
		func(item port.UpdatePredicate) string { return string(item.Operator) },
		func(item port.UpdatePredicate) any { return item.Value },
		func(item port.UpdatePredicate) []any { return item.Values },
		func(field port.FieldKey) (string, bool) { return columnFor(field) },
		func(field port.FieldKey, value any) (any, error) { return valueFor(field, value) },
	)
}

func buildUpdateAssignments(
	assignments port.UpdateAssignments,
	columnFor updateFieldColumnResolver,
	valueFor updateFieldValueMapper,
) (map[string]any, error) {
	return postgrescommon.BuildUpdateAssignments(
		assignments,
		func(item port.UpdateAssignment) port.FieldKey { return item.Field },
		func(item port.UpdateAssignment) any { return item.Value },
		func(field port.FieldKey) (string, bool) { return columnFor(field) },
		func(field port.FieldKey, value any) (any, error) { return valueFor(field, value) },
	)
}
