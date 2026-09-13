package cypher

import (
	"context"
	"strings"

	"github.com/orneryd/nornicdb/pkg/storage"
)

func splitPatternComprehension(expr string) (string, string, bool) {
	expr = strings.TrimSpace(expr)
	if len(expr) < 3 || expr[0] != '[' || expr[len(expr)-1] != ']' {
		return "", "", false
	}

	parenDepth, bracketDepth, braceDepth := 0, 1, 0
	var quote byte
	for i := 1; i < len(expr)-1; i++ {
		char := expr[i]
		if quote != 0 {
			if char == '\\' {
				i++
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '|':
			if parenDepth == 0 && bracketDepth == 1 && braceDepth == 0 {
				pattern := strings.TrimSpace(expr[1:i])
				projection := strings.TrimSpace(expr[i+1 : len(expr)-1])
				if !strings.Contains(strings.ToUpper(pattern), " IN ") &&
					strings.Contains(pattern, "(") && strings.Contains(pattern, ")") && projection != "" {
					return pattern, projection, true
				}
			}
		}
	}
	return "", "", false
}

func standaloneCountSubquery(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if !hasSubqueryPattern(expr, countSubqueryRe) {
		return "", false
	}
	open := strings.Index(expr, "{")
	close := strings.LastIndex(expr, "}")
	if open < 0 || close <= open || strings.TrimSpace(expr[close+1:]) != "" {
		return "", false
	}
	return strings.TrimSpace(expr[open+1 : close]), true
}

func (e *StorageExecutor) evaluateBoundPatternRows(ctx context.Context, pattern string, nodes map[string]*storage.Node, rels map[string]*storage.Edge) []traversalOptRow {
	pattern = strings.TrimSpace(pattern)
	if strings.HasPrefix(strings.ToUpper(pattern), "MATCH ") {
		pattern = strings.TrimSpace(pattern[len("MATCH "):])
	}
	clauses := splitOptionalMatchClauses(pattern)
	if len(clauses) != 1 {
		return nil
	}
	seed := traversalOptRow{
		nodes: make(map[string]*storage.Node, len(nodes)),
		rels:  make(map[string]*storage.Edge, len(rels)),
	}
	for name, node := range nodes {
		seed.nodes[name] = node
	}
	for name, relationship := range rels {
		seed.rels[name] = relationship
	}

	expanded, err := e.applyTraversalOptionalClause(ctx, []traversalOptRow{seed}, clauses[0])
	if err != nil {
		return nil
	}
	matches := expanded[:0]
	for _, row := range expanded {
		if row.optionalMatched {
			matches = append(matches, row)
		}
	}
	return matches
}

func (e *StorageExecutor) evaluatePatternComprehension(ctx context.Context, pattern, projection string, nodes map[string]*storage.Node, rels map[string]*storage.Edge) []interface{} {
	rows := e.evaluateBoundPatternRows(ctx, pattern, nodes, rels)
	values := make([]interface{}, 0, len(rows))
	for _, row := range rows {
		values = append(values, e.evaluateExpressionWithContext(ctx, projection, row.nodes, row.rels))
	}
	return values
}
