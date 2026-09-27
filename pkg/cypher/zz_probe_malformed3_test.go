package cypher

import (
	"context"
	"fmt"
	"testing"
)

func TestZZProbeMalformed3(t *testing.T) {
	exec, _ := newTestExecutor(t)
	ctx := context.Background()
	cases := []string{
		"RETURN 1 LIMIT 'x'",
		"RETURN 1 SKIP 'x'",
		"RETURN 1 ORDER BY n.x",
		"MATCH (n) RETURN n LIMIT -1",
		"CALL { RETURN 1 AS x } RETURN x",
		"CALL { RETURN 1 AS x } RETURN",
		"RETURN 1 AS x // c",
		"UNWIND [1, 2] AS x RETURN x SKIP",
		"WITH 1 AS x WHERE RETURN x",
		"MATCH (n) WHERE n.x IN RETURN n",
	}
	for _, query := range cases {
		result, err := exec.Execute(ctx, query, nil)
		if err != nil {
			fmt.Printf("QUERY: %q\n  ERROR: %v\n", query, err)
			continue
		}
		fmt.Printf("QUERY: %q\n  OK COLS: %v ROWS: %v\n", query, result.Columns, result.Rows)
	}
}
