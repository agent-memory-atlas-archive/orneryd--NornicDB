package cypher

import (
	"context"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestRuntimeExpressionFailuresReturnErrors(t *testing.T) {
	for _, query := range []string{
		"RETURN 1 + {a: 1} AS x",
		"RETURN true + 1 AS x",
		"RETURN 'a' - 1 AS x",
		"RETURN [1] * 2 AS x",
		"RETURN 1 / 0 AS x",
		"RETURN 1 % 0 AS x",
		"RETURN labels('x') AS x",
		"RETURN date('not a date') AS x",
		"RETURN substring('abc', -1) AS x",
	} {
		t.Run(query, func(t *testing.T) {
			exec := NewStorageExecutor(storage.NewNamespacedEngine(newTestMemoryEngine(t), "test"))
			_, err := exec.Execute(context.Background(), query, nil)
			require.Error(t, err)
		})
	}
}

func TestRuntimeExpressionFailuresRollbackAllWrites(t *testing.T) {
	for _, query := range []string{
		"MATCH (p:P) SET p.a = 1, p.b = 1 / 0 RETURN p.a AS a",
		"MATCH (p:P) SET p.a = 1, p.b = date('x') RETURN p.a AS a",
		"UNWIND [1, 0] AS d CREATE (:W {v: 1 / d})",
	} {
		t.Run(query, func(t *testing.T) {
			engine := storage.NewNamespacedEngine(newTestMemoryEngine(t), "test")
			exec := NewStorageExecutor(engine)
			_, err := exec.Execute(context.Background(), "CREATE (:P {id: 1, name: 'a'})", nil)
			require.NoError(t, err)
			_, err = exec.Execute(context.Background(), query, nil)
			require.Error(t, err)
			nodes, err := engine.AllNodes()
			require.NoError(t, err)
			require.Len(t, nodes, 1)
			require.Equal(t, map[string]interface{}{"id": int64(1), "name": "a"}, nodes[0].Properties)
		})
	}
}

func TestRuntimeExpressionFailureExplicitTransaction(t *testing.T) {
	for _, query := range []string{"CREATE (:W {v: 1 / 0})", "RETURN true + 1 AS x", "RETURN date('bad') AS x"} {
		t.Run(query, func(t *testing.T) {
			engine := storage.NewNamespacedEngine(newTestMemoryEngine(t), "test")
			exec := NewStorageExecutor(engine)
			ctx := context.Background()
			_, err := exec.Execute(ctx, "BEGIN", nil)
			require.NoError(t, err)
			_, err = exec.Execute(ctx, "CREATE (:W {v: 1})", nil)
			require.NoError(t, err)
			_, err = exec.Execute(ctx, query, nil)
			require.Error(t, err)
			_, err = exec.Execute(ctx, "COMMIT", nil)
			require.Error(t, err)
			nodes, err := engine.AllNodes()
			require.NoError(t, err)
			require.Empty(t, nodes)
		})
	}
}

func TestRuntimeNullArithmeticRemainsNull(t *testing.T) {
	exec := NewStorageExecutor(storage.NewNamespacedEngine(newTestMemoryEngine(t), "test"))
	for _, query := range []string{"RETURN null + 1 AS x", "RETURN 1 / null AS x", "RETURN null % 0 AS x"} {
		t.Run(query, func(t *testing.T) {
			result, err := exec.Execute(context.Background(), query, nil)
			require.NoError(t, err)
			require.Nil(t, result.Rows[0][0])
		})
	}
	_, err := exec.Execute(context.Background(), "CREATE (:W {v: 1 / null})", nil)
	require.NoError(t, err)
	nodes, err := exec.storage.AllNodes()
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Nil(t, nodes[0].Properties["v"])
}
