package search

import (
	"path/filepath"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestLiveVectorIndexingCreatesConfiguredFileStore(t *testing.T) {
	vectorPath := filepath.Join(t.TempDir(), "vectors")
	svc := NewServiceWithDimensions(storage.NewMemoryEngine(), 2)
	t.Cleanup(func() { require.NoError(t, svc.Close()) })
	svc.SetVectorIndexPath(vectorPath)
	svc.SetPersistenceEnabled(true)

	require.NoError(t, svc.IndexNode(&storage.Node{
		ID:              "first-live-node",
		ChunkEmbeddings: [][]float32{{1, 0}},
	}))

	require.NotNil(t, svc.vectorFileStore)
	require.Equal(t, 1, svc.vectorFileStore.Count())
	_, ok := svc.vectorFileStore.GetVector("first-live-node")
	require.True(t, ok)
	require.FileExists(t, vectorPath+".vec")
	require.NoError(t, svc.persistBaseIndexes())
	require.FileExists(t, vectorPath+".meta")

	svc.vectorIndex.mu.RLock()
	require.Empty(t, svc.vectorIndex.vectors)
	require.Empty(t, svc.vectorIndex.rawVectors)
	svc.vectorIndex.mu.RUnlock()
}

func TestEnablingVectorPersistenceMigratesExistingLiveVectors(t *testing.T) {
	vectorPath := filepath.Join(t.TempDir(), "vectors")
	svc := NewServiceWithDimensions(storage.NewMemoryEngine(), 2)
	t.Cleanup(func() { require.NoError(t, svc.Close()) })

	require.NoError(t, svc.IndexNode(&storage.Node{
		ID:              "before-persistence",
		ChunkEmbeddings: [][]float32{{0, 1}},
	}))
	require.Equal(t, 1, svc.vectorIndex.Count())

	svc.SetVectorIndexPath(vectorPath)
	svc.SetPersistenceEnabled(true)
	require.NoError(t, svc.IndexNode(&storage.Node{
		ID:              "after-persistence",
		ChunkEmbeddings: [][]float32{{1, 0}},
	}))

	require.NotNil(t, svc.vectorFileStore)
	require.Equal(t, 2, svc.vectorFileStore.Count())
	for _, id := range []string{"before-persistence", "after-persistence"} {
		_, ok := svc.vectorFileStore.GetVector(id)
		require.True(t, ok, "missing migrated vector %q", id)
	}
	require.Zero(t, svc.vectorIndex.Count())
}
