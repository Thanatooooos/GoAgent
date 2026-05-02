package common

import (
	"fmt"

	"gorm.io/gorm"
)

const (
	operatorEQ        = "eq"
	operatorNE        = "ne"
	operatorLT        = "lt"
	operatorLTE       = "lte"
	operatorGT        = "gt"
	operatorGTE       = "gte"
	operatorIn        = "in"
	operatorIsNull    = "is_null"
	operatorIsNotNull = "is_not_null"
)

type UpdateFieldColumnResolver[F comparable] func(F) (string, bool)

type UpdateFieldValueMapper[F comparable] func(F, any) (any, error)

// ConditionalUpdateRequiresConditions 统一生成条件更新缺少 where 条件的错误。
func ConditionalUpdateRequiresConditions(entity string) error {
	return fmt.Errorf("update %s with conditions: conditions are required", entity)
}

// ApplyUpdatePredicates 将通用更新谓词转为 GORM where 条件。
func ApplyUpdatePredicates[F comparable, P any](
	query *gorm.DB,
	predicates []P,
	fieldOf func(P) F,
	operatorOf func(P) string,
	valueOf func(P) any,
	valuesOf func(P) []any,
	columnFor UpdateFieldColumnResolver[F],
	valueFor UpdateFieldValueMapper[F],
) (*gorm.DB, error) {
	for _, predicate := range predicates {
		field := fieldOf(predicate)
		column, ok := columnFor(field)
		if !ok {
			return nil, fmt.Errorf("unsupported update condition field: %v", field)
		}

		switch operatorOf(predicate) {
		case operatorEQ:
			value, err := valueFor(field, valueOf(predicate))
			if err != nil {
				return nil, err
			}
			query = query.Where(column+" = ?", value)
		case operatorNE:
			value, err := valueFor(field, valueOf(predicate))
			if err != nil {
				return nil, err
			}
			query = query.Where(column+" <> ?", value)
		case operatorLT:
			value, err := valueFor(field, valueOf(predicate))
			if err != nil {
				return nil, err
			}
			query = query.Where(column+" < ?", value)
		case operatorLTE:
			value, err := valueFor(field, valueOf(predicate))
			if err != nil {
				return nil, err
			}
			query = query.Where(column+" <= ?", value)
		case operatorGT:
			value, err := valueFor(field, valueOf(predicate))
			if err != nil {
				return nil, err
			}
			query = query.Where(column+" > ?", value)
		case operatorGTE:
			value, err := valueFor(field, valueOf(predicate))
			if err != nil {
				return nil, err
			}
			query = query.Where(column+" >= ?", value)
		case operatorIn:
			values, err := predicateValues(field, valuesOf(predicate), valueFor)
			if err != nil {
				return nil, err
			}
			query = query.Where(column+" IN ?", values)
		case operatorIsNull:
			query = query.Where(column + " IS NULL")
		case operatorIsNotNull:
			query = query.Where(column + " IS NOT NULL")
		default:
			return nil, fmt.Errorf("unsupported update condition operator: %s", operatorOf(predicate))
		}
	}
	return query, nil
}

// BuildUpdateAssignments 将通用字段赋值转为 GORM updates map。
func BuildUpdateAssignments[F comparable, A any](
	assignments []A,
	fieldOf func(A) F,
	valueOf func(A) any,
	columnFor UpdateFieldColumnResolver[F],
	valueFor UpdateFieldValueMapper[F],
) (map[string]any, error) {
	updates := map[string]any{}
	for _, assignment := range assignments {
		field := fieldOf(assignment)
		column, ok := columnFor(field)
		if !ok {
			return nil, fmt.Errorf("unsupported update assignment field: %v", field)
		}

		value, err := valueFor(field, valueOf(assignment))
		if err != nil {
			return nil, err
		}
		updates[column] = value
	}
	return updates, nil
}

func predicateValues[F comparable](
	field F,
	items []any,
	valueFor UpdateFieldValueMapper[F],
) ([]any, error) {
	values := make([]any, 0, len(items))
	for _, item := range items {
		value, err := valueFor(field, item)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
