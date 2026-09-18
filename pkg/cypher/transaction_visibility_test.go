package cypher

import (
	"context"
	"testing"
	"time"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestExplicitTransactionSeesAcknowledgedAsyncWrite(t *testing.T) {
	base := newTestMemoryEngine(t)
	async := storage.NewAsyncEngine(base, &storage.AsyncEngineConfig{
		FlushInterval:    time.Hour,
		MaxNodeCacheSize: 1000,
		MaxEdgeCacheSize: 1000,
	})
	t.Cleanup(func() { require.NoError(t, async.Close()) })
	executor := NewStorageExecutor(storage.NewNamespacedEngine(async, "visibility"))

	created, err := executor.Execute(context.Background(),
		"CREATE (:Account {accountID: 'acknowledged'})", nil)
	require.NoError(t, err)
	require.Equal(t, 1, created.Stats.NodesCreated)
	require.True(t, async.HasPendingWrites(), "the reproduction requires an unflushed acknowledged write")

	_, err = executor.handleBegin()
	require.NoError(t, err)
	t.Cleanup(func() {
		if executor.txContext != nil && executor.txContext.active {
			_, _ = executor.handleRollback()
		}
	})

	matched, err := executor.Execute(context.Background(),
		"MATCH (account:Account {accountID: 'acknowledged'}) RETURN account.accountID", nil)
	require.NoError(t, err)
	require.Equal(t, [][]interface{}{{"acknowledged"}}, matched.Rows)
}

func TestExplicitTransactionCreatesDependentWritesFromAcknowledgedState(t *testing.T) {
	base := newTestMemoryEngine(t)
	async := storage.NewAsyncEngine(base, &storage.AsyncEngineConfig{
		FlushInterval:    time.Hour,
		MaxNodeCacheSize: 1000,
		MaxEdgeCacheSize: 1000,
	})
	t.Cleanup(func() { require.NoError(t, async.Close()) })
	executor := NewStorageExecutor(storage.NewNamespacedEngine(async, "dependent"))

	_, err := executor.Execute(context.Background(),
		"CREATE (:Account {accountID: 'source'})", nil)
	require.NoError(t, err)
	_, err = executor.handleBegin()
	require.NoError(t, err)

	created, err := executor.Execute(context.Background(), `
		MATCH (source:Account {accountID: 'source'})
		CREATE (target:Account {accountID: 'target'})
		CREATE (source)-[:LINKS_TO]->(target)
	`, nil)
	require.NoError(t, err)
	require.Equal(t, 1, created.Stats.RelationshipsCreated)
	_, err = executor.handleCommit()
	require.NoError(t, err)

	result, err := executor.Execute(context.Background(),
		"MATCH (:Account {accountID: 'source'})-[r:LINKS_TO]->(:Account {accountID: 'target'}) RETURN count(r)", nil)
	require.NoError(t, err)
	require.Equal(t, [][]interface{}{{int64(1)}}, result.Rows)
}
