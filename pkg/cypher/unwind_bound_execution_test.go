package cypher

import (
	"context"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

// TestUnwind_MatchedPerItemBoundExecution pins the UNWIND migration (§6.2):
// unwound rows travel as value bindings + parameters, never as query-text
// substitution, so hostile values (quotes, punctuation) and map property
// access resolve with their real Go values through the per-item MATCH path.
func TestUnwind_MatchedPerItemBoundExecution(t *testing.T) {
	baseStore := newTestMemoryEngine(t)
	store := storage.NewNamespacedEngine(baseStore, "test")
	exec := NewStorageExecutor(store)
	ctx := context.Background()

	_, err := exec.Execute(ctx, `CREATE (:Customer {customerID: 1})`, nil)
	require.NoError(t, err)

	_, err = exec.Execute(ctx, `
UNWIND $rows AS row
MATCH (c:Customer {customerID: row.customerID})
CREATE (o:Order {orderID: row.orderID, note: row.notes})
RETURN o.orderID AS id, o.note AS note
ORDER BY id
`, map[string]interface{}{
		"rows": []interface{}{
			map[string]interface{}{"customerID": int64(1), "orderID": int64(9001), "notes": "O'Brien;2"},
			map[string]interface{}{"customerID": int64(1), "orderID": int64(9002), "notes": "a, b"},
		},
	})
	require.NoError(t, err)

	got, err := exec.Execute(ctx, `MATCH (o:Order) RETURN o.orderID, o.note ORDER BY o.orderID`, nil)
	require.NoError(t, err)
	require.Len(t, got.Rows, 2)
	require.EqualValues(t, 9001, got.Rows[0][0])
	require.Equal(t, "O'Brien;2", got.Rows[0][1])
	require.EqualValues(t, 9002, got.Rows[1][0])
	require.Equal(t, "a, b", got.Rows[1][1])
}
