package cypher

import (
	"context"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

// TestForeach_BoundExecutionPinsNoSubstitution pins the FOREACH flip (§6.2):
// the loop variable travels as a value binding, so values that are hostile to
// query-text substitution (quotes, identifier-like content) and structured
// map items resolve with their real Go values.
func TestForeach_BoundExecutionPinsNoSubstitution(t *testing.T) {
	baseStore := newTestMemoryEngine(t)
	store := storage.NewNamespacedEngine(baseStore, "test")
	exec := NewStorageExecutor(store)
	ctx := context.Background()

	// Quote-heavy string values survive without text re-entry.
	_, err := exec.Execute(ctx, `FOREACH (x IN ["O'Brien", "a;b"] | CREATE (:QuoteSafe {v: x}))`, nil)
	require.NoError(t, err)
	got, err := exec.Execute(ctx, `MATCH (n:QuoteSafe) RETURN n.v ORDER BY n.v`, nil)
	require.NoError(t, err)
	require.Len(t, got.Rows, 2)
	require.Equal(t, "O'Brien", got.Rows[0][0])
	require.Equal(t, "a;b", got.Rows[1][0])

	// Structured map items resolve through dotted access.
	_, err = exec.Execute(ctx, `FOREACH (x IN [{name: 'ann', age: 3}] | CREATE (:MapItem {name: x.name, age: x.age}))`, nil)
	require.NoError(t, err)
	got, err = exec.Execute(ctx, `MATCH (n:MapItem) RETURN n.name, n.age`, nil)
	require.NoError(t, err)
	require.Len(t, got.Rows, 1)
	require.Equal(t, "ann", got.Rows[0][0])
	require.EqualValues(t, 3, got.Rows[0][1])

	// Typed integer loop values keep their numeric type.
	_, err = exec.Execute(ctx, `FOREACH (x IN [7] | CREATE (:Typed {v: x}))`, nil)
	require.NoError(t, err)
	got, err = exec.Execute(ctx, `MATCH (n:Typed) RETURN n.v`, nil)
	require.NoError(t, err)
	require.Len(t, got.Rows, 1)
	require.EqualValues(t, 7, got.Rows[0][0])
}
