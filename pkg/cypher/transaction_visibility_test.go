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

func TestOpenExplicitTransactionDoesNotBlockAnotherBeginAfterAcknowledgedWrite(t *testing.T) {
	base := newTestMemoryEngine(t)
	async := storage.NewAsyncEngine(base, &storage.AsyncEngineConfig{
		FlushInterval:    time.Hour,
		MaxNodeCacheSize: 1000,
		MaxEdgeCacheSize: 1000,
	})
	t.Cleanup(func() { require.NoError(t, async.Close()) })
	store := storage.NewNamespacedEngine(async, "concurrent_begin")
	first := NewStorageExecutor(store)
	second := NewStorageExecutor(store)

	_, err := first.handleBegin()
	require.NoError(t, err)
	firstOpen := true
	t.Cleanup(func() {
		if firstOpen {
			_, _ = first.handleRollback()
		}
		if second.txContext != nil && second.txContext.active {
			_, _ = second.handleRollback()
		}
	})

	_, err = async.CreateNode(&storage.Node{
		ID:     "concurrent_begin:pending",
		Labels: []string{"Pending"},
	})
	require.NoError(t, err)
	require.True(t, async.HasPendingWrites(), "the reproduction requires a pending acknowledged write")

	beginDone := make(chan error, 1)
	go func() {
		_, beginErr := second.handleBegin()
		beginDone <- beginErr
	}()

	var beginErr error
	beginCompletedPromptly := false
	select {
	case beginErr = <-beginDone:
		beginCompletedPromptly = true
	case <-time.After(250 * time.Millisecond):
	}

	// Always release the first transaction before asserting so a regression
	// cannot strand the second BEGIN goroutine or the async engine cleanup.
	_, rollbackErr := first.handleRollback()
	require.NoError(t, rollbackErr)
	firstOpen = false
	if !beginCompletedPromptly {
		select {
		case beginErr = <-beginDone:
		case <-time.After(time.Second):
			t.Fatal("second BEGIN remained blocked after the first transaction rolled back")
		}
	}
	require.True(t, beginCompletedPromptly, "second BEGIN blocked for the lifetime of an unrelated explicit transaction")
	require.NoError(t, beginErr)
}

func TestCountReadDoesNotStallWhileAnotherTransactionBegins(t *testing.T) {
	base := newTestMemoryEngine(t)
	async := storage.NewAsyncEngine(base, &storage.AsyncEngineConfig{
		FlushInterval:    time.Hour,
		MaxNodeCacheSize: 1000,
		MaxEdgeCacheSize: 1000,
	})
	t.Cleanup(func() { require.NoError(t, async.Close()) })
	store := storage.NewNamespacedEngine(async, "concurrent_count")
	first := NewStorageExecutor(store)
	second := NewStorageExecutor(store)

	_, err := first.handleBegin()
	require.NoError(t, err)
	t.Cleanup(func() {
		if first.txContext != nil && first.txContext.active {
			_, _ = first.handleRollback()
		}
		if second.txContext != nil && second.txContext.active {
			_, _ = second.handleRollback()
		}
	})

	_, err = async.CreateNode(&storage.Node{
		ID:     "concurrent_count:pending",
		Labels: []string{"Pending"},
	})
	require.NoError(t, err)

	beginDone := make(chan error, 1)
	go func() {
		_, beginErr := second.handleBegin()
		beginDone <- beginErr
	}()

	countDone := make(chan error, 1)
	go func() {
		_, countErr := async.NodeCount()
		countDone <- countErr
	}()

	select {
	case countErr := <-countDone:
		require.NoError(t, countErr)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("count read stalled behind transaction snapshot admission")
	}
	select {
	case beginErr := <-beginDone:
		require.NoError(t, beginErr)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("second BEGIN stalled behind an open transaction")
	}
}
