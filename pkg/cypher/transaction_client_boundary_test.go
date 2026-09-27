package cypher

import (
	"context"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestClientTransactionControlPreservesOwnedTransaction(t *testing.T) {
	store := storage.NewNamespacedEngine(storage.NewMemoryEngine(), "client_boundary")
	executor := NewStorageExecutor(store)
	ctx := context.Background()
	client := WithClientStatement(ctx)
	owner := WithTransactionControl(client)

	_, err := executor.Execute(owner, "BEGIN", nil)
	require.NoError(t, err)
	_, err = executor.Execute(client, "CREATE (:ClientBoundary)", nil)
	require.NoError(t, err)
	for _, command := range []string{"COMMIT", "ROLLBACK TRANSACTION", "BEGIN"} {
		_, err = executor.Execute(client, command, nil)
		var syntax *SemanticError
		require.ErrorAs(t, err, &syntax, command)
		require.Equal(t, "Neo.ClientError.Statement.SyntaxError", syntax.Code)
		require.True(t, executor.HasActiveTransaction(), command)
	}
	_, err = executor.Execute(owner, "COMMIT", nil)
	require.NoError(t, err)
	require.False(t, executor.HasActiveTransaction())
	result, err := executor.Execute(client, "MATCH (n:ClientBoundary) RETURN count(n)", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Rows[0][0])
}
