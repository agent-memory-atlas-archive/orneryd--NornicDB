package cypher

import (
	"context"
	"strings"

	"github.com/orneryd/nornicdb/pkg/storage"
)

// evaluateRowExpression resolves an expression against a heterogeneous Cypher
// row. Unlike the graph-only evaluator, a row may also contain scalar, map,
// and list bindings introduced by WITH or UNWIND.
func (e *StorageExecutor) evaluateRowExpression(expr string, values map[string]interface{}) (interface{}, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, false
	}
	if value, ok := values[expr]; ok {
		return value, true
	}
	if value, ok := parseLiteralValueFromComputedRow(expr); ok {
		return value, true
	}

	if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") {
		inner := strings.TrimSpace(expr[1 : len(expr)-1])
		if inner == "" {
			return []interface{}{}, true
		}
		items := splitTopLevelComma(inner)
		result := make([]interface{}, 0, len(items))
		for _, item := range items {
			value, ok := e.evaluateRowExpression(item, values)
			if !ok {
				return nil, false
			}
			result = append(result, value)
		}
		return result, true
	}

	if open := strings.LastIndex(expr, "["); open > 0 && strings.HasSuffix(expr, "]") {
		base, ok := e.evaluateRowExpression(expr[:open], values)
		if !ok {
			return nil, false
		}
		indexValue, ok := e.evaluateRowExpression(expr[open+1:len(expr)-1], values)
		if !ok {
			return nil, false
		}
		index, ok := rowSubscriptIndex(indexValue)
		if !ok {
			return nil, false
		}
		items := toAnySlice(base)
		if index < 0 {
			index += len(items)
		}
		if index < 0 || index >= len(items) {
			return nil, true
		}
		return items[index], true
	}

	if left, right, ok := splitByOperatorWithOptions(expr, "+", true, false); ok {
		leftValue, leftOK := e.evaluateRowExpression(left, values)
		rightValue, rightOK := e.evaluateRowExpression(right, values)
		if !leftOK || !rightOK {
			return nil, false
		}
		if leftText, ok := leftValue.(string); ok {
			if rightText, ok := rightValue.(string); ok {
				return leftText + rightText, true
			}
		}
		return e.add(leftValue, rightValue), true
	}

	if dot := strings.Index(expr, "."); dot > 0 {
		baseName := strings.TrimSpace(expr[:dot])
		property := strings.TrimSpace(expr[dot+1:])
		if base, exists := values[baseName]; exists {
			switch value := base.(type) {
			case *storage.Node:
				if value == nil {
					return nil, true
				}
				return value.Properties[property], true
			case *storage.Edge:
				if value == nil {
					return nil, true
				}
				return value.Properties[property], true
			default:
				if object, ok := toStringAnyMap(base); ok {
					return object[property], true
				}
			}
		}
	}
	value := e.evaluateExpressionFromValues(expr, values)
	if text, ok := value.(string); ok && text == expr && !isWholeCypherQuotedString(expr) {
		return nil, false
	}
	return value, true
}

func rowSubscriptIndex(value interface{}) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		integer := int(number)
		return integer, float64(integer) == number
	default:
		return 0, false
	}
}

func (e *StorageExecutor) evaluateRowPredicate(ctx context.Context, expression string, values map[string]interface{}) bool {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return false
	}
	if variable, labels, ok := parseWithWhereLabelTest(expression); ok {
		return withWhereNodeHasAllLabels(values[variable], labels)
	}
	if left, right, ok := splitByOperatorWithOptions(expression, " AND ", true, false); ok {
		return e.evaluateRowPredicate(ctx, left, values) && e.evaluateRowPredicate(ctx, right, values)
	}
	if left, right, ok := splitByOperatorWithOptions(expression, " OR ", true, false); ok {
		return e.evaluateRowPredicate(ctx, left, values) || e.evaluateRowPredicate(ctx, right, values)
	}
	if hasPrefixFoldASCII(expression, "NOT ") {
		return !e.evaluateRowPredicate(ctx, strings.TrimSpace(expression[4:]), values)
	}
	if left, right, ok := splitByOperatorWithOptions(expression, " STARTS WITH ", true, true); ok {
		return e.evaluateRowStringPredicate(left, right, values, strings.HasPrefix)
	}
	if left, right, ok := splitByOperatorWithOptions(expression, " ENDS WITH ", true, true); ok {
		return e.evaluateRowStringPredicate(left, right, values, strings.HasSuffix)
	}
	if left, right, ok := splitByOperatorWithOptions(expression, " CONTAINS ", true, true); ok {
		return e.evaluateRowStringPredicate(left, right, values, strings.Contains)
	}
	if left, right, ok := splitByOperatorWithOptions(expression, " IN ", true, true); ok {
		needle, leftOK := e.evaluateRowExpression(left, values)
		haystack, rightOK := e.evaluateRowExpression(right, values)
		if !leftOK || !rightOK {
			return false
		}
		for _, item := range toAnySlice(haystack) {
			if e.compareEqual(needle, item) {
				return true
			}
		}
		return false
	}
	for _, operator := range []string{" IS NOT NULL", " IS NULL"} {
		if hasSuffixFoldASCII(expression, strings.ToLower(operator)) {
			left := strings.TrimSpace(expression[:len(expression)-len(operator)])
			value, ok := e.evaluateRowExpression(left, values)
			if operator == " IS NULL" {
				return !ok || value == nil
			}
			return ok && value != nil
		}
	}
	for _, operator := range []string{"<=", ">=", "<>", "!=", "=", "<", ">"} {
		if left, right, ok := splitByOperatorWithOptions(expression, operator, true, true); ok {
			leftValue, leftOK := e.evaluateRowExpression(left, values)
			rightValue, rightOK := e.evaluateRowExpression(right, values)
			if !leftOK || !rightOK {
				return false
			}
			if operator == "!=" {
				operator = "<>"
			}
			return compareWithOperator(leftValue, rightValue, operator)
		}
	}
	value, ok := e.evaluateRowExpression(expression, values)
	return ok && isTruthy(value)
}

func (e *StorageExecutor) evaluateRowStringPredicate(left, right string, values map[string]interface{}, predicate func(string, string) bool) bool {
	leftValue, leftOK := e.evaluateRowExpression(left, values)
	rightValue, rightOK := e.evaluateRowExpression(right, values)
	leftText, leftString := leftValue.(string)
	rightText, rightString := rightValue.(string)
	return leftOK && rightOK && leftString && rightString && predicate(leftText, rightText)
}
