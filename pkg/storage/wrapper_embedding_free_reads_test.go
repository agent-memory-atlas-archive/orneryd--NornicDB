package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type embeddingFreeReadSpy struct {
	Engine
	fullReads       int
	fullBatchReads  int
	lightReads      int
	lightBatchReads int
}

func (s *embeddingFreeReadSpy) GetNode(id NodeID) (*Node, error) {
	s.fullReads++
	return s.Engine.GetNode(id)
}

func (s *embeddingFreeReadSpy) BatchGetNodes(ids []NodeID) (map[NodeID]*Node, error) {
	s.fullBatchReads++
	return s.Engine.BatchGetNodes(ids)
}

func (s *embeddingFreeReadSpy) GetNodeWithoutEmbeddings(id NodeID) (*Node, error) {
	s.lightReads++
	return s.Engine.(NodeWithoutEmbeddingsReader).GetNodeWithoutEmbeddings(id)
}

func (s *embeddingFreeReadSpy) BatchGetNodesWithoutEmbeddings(ids []NodeID) (map[NodeID]*Node, error) {
	s.lightBatchReads++
	return s.Engine.(BatchNodeWithoutEmbeddingsReader).BatchGetNodesWithoutEmbeddings(ids)
}

func TestEmbeddingFreeBatchReadsTraverseNamespacedAsyncWALStack(t *testing.T) {
	badger := createTestBadgerEngine(t)
	spy := &embeddingFreeReadSpy{Engine: badger}
	walLog, err := NewWAL(t.TempDir(), &WALConfig{SyncMode: "none"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, walLog.Close()) })
	wal := NewWALEngine(spy, walLog)
	async := NewAsyncEngine(wal, &AsyncEngineConfig{
		FlushInterval:    time.Hour,
		MaxNodeCacheSize: 1000,
		MaxEdgeCacheSize: 1000,
	})
	t.Cleanup(func() { require.NoError(t, async.Close()) })
	tenant := NewNamespacedEngine(async, "library")

	node := &Node{
		ID:         "persisted",
		Labels:     []string{"Transcript"},
		Properties: map[string]any{"state": "ready"},
		ChunkEmbeddings: [][]float32{
			make([]float32, 4096),
			make([]float32, 4096),
		},
		NamedEmbeddings: map[string][]float32{"visual": make([]float32, 4096)},
	}
	_, err = tenant.CreateNode(node)
	require.NoError(t, err)
	require.NoError(t, async.Flush())
	pending := &Node{
		ID:              "pending",
		Labels:          []string{"Transcript"},
		Properties:      map[string]any{"state": "queued"},
		ChunkEmbeddings: [][]float32{make([]float32, 4096)},
	}
	_, err = tenant.CreateNode(pending)
	require.NoError(t, err)

	require.True(t, tenant.BatchGetNodesWithoutEmbeddingsSupported())
	light, err := tenant.BatchGetNodesWithoutEmbeddings([]NodeID{node.ID, pending.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"Transcript"}, light[node.ID].Labels)
	require.Equal(t, "ready", light[node.ID].Properties["state"])
	require.Empty(t, light[node.ID].ChunkEmbeddings)
	require.Empty(t, light[node.ID].NamedEmbeddings)
	require.Equal(t, "queued", light[pending.ID].Properties["state"])
	require.Empty(t, light[pending.ID].ChunkEmbeddings)
	require.Equal(t, 1, spy.lightBatchReads)
	require.Zero(t, spy.lightReads)
	require.Zero(t, spy.fullBatchReads, "wrapper stack must not load full embedding-bearing nodes")
	require.Zero(t, spy.fullReads, "wrapper stack must not load full embedding-bearing nodes")
}

func TestEmbeddingFreeSingleReadsTraverseAsyncWALStack(t *testing.T) {
	badger := createTestBadgerEngine(t)
	spy := &embeddingFreeReadSpy{Engine: badger}
	walLog, err := NewWAL(t.TempDir(), &WALConfig{SyncMode: "none"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, walLog.Close()) })
	async := NewAsyncEngine(NewWALEngine(spy, walLog), &AsyncEngineConfig{FlushInterval: time.Hour})
	t.Cleanup(func() { require.NoError(t, async.Close()) })

	node := &Node{ID: "nornic:persisted", ChunkEmbeddings: [][]float32{make([]float32, 4096)}}
	_, err = async.CreateNode(node)
	require.NoError(t, err)
	require.NoError(t, async.Flush())

	light, err := async.GetNodeWithoutEmbeddings(node.ID)
	require.NoError(t, err)
	require.Empty(t, light.ChunkEmbeddings)
	require.Equal(t, 1, spy.lightReads)
	require.Zero(t, spy.fullReads)
}

func TestEmbeddingFreeBatchCapabilityRejectsUnsupportedInnerEngine(t *testing.T) {
	base := newEngineWithoutEmbeddingFreeReads(t)
	walLog, err := NewWAL(t.TempDir(), &WALConfig{SyncMode: "none"})
	require.NoError(t, err)
	wal := NewWALEngine(base, walLog)
	async := NewAsyncEngine(wal, &AsyncEngineConfig{FlushInterval: time.Hour})
	t.Cleanup(func() { require.NoError(t, async.Close()) })
	tenant := NewNamespacedEngine(async, "unsupported")

	require.False(t, wal.BatchGetNodesWithoutEmbeddingsSupported())
	require.False(t, async.BatchGetNodesWithoutEmbeddingsSupported())
	require.False(t, tenant.BatchGetNodesWithoutEmbeddingsSupported())
	_, err = tenant.CreateNode(&Node{
		ID:              "cached",
		ChunkEmbeddings: [][]float32{make([]float32, 4096)},
	})
	require.NoError(t, err)
	cached, err := tenant.BatchGetNodesWithoutEmbeddings([]NodeID{"cached"})
	require.NoError(t, err)
	require.Empty(t, cached["cached"].ChunkEmbeddings)
	_, err = async.BatchGetNodesWithoutEmbeddings([]NodeID{"missing"})
	require.ErrorIs(t, err, ErrNotImplemented)
}

type engineWithoutEmbeddingFreeReads struct{ Engine }

func newEngineWithoutEmbeddingFreeReads(t *testing.T) Engine {
	t.Helper()
	base := NewMemoryEngine()
	t.Cleanup(func() { require.NoError(t, base.Close()) })
	return &engineWithoutEmbeddingFreeReads{Engine: base}
}

func BenchmarkNamespacedAsyncWALBatchNodeReads(b *testing.B) {
	badger, err := NewBadgerEngineInMemory()
	require.NoError(b, err)
	walLog, err := NewWAL(b.TempDir(), &WALConfig{SyncMode: "none"})
	require.NoError(b, err)
	async := NewAsyncEngine(NewWALEngine(badger, walLog), &AsyncEngineConfig{FlushInterval: time.Hour})
	b.Cleanup(func() { require.NoError(b, async.Close()) })
	tenant := NewNamespacedEngine(async, "benchmark")

	ids := make([]NodeID, 16)
	for index := range ids {
		ids[index] = NodeID("node-" + string(rune('a'+index)))
		node := &Node{
			ID:         ids[index],
			Labels:     []string{"Transcript"},
			Properties: map[string]any{"state": "ready"},
		}
		for chunk := 0; chunk < 8; chunk++ {
			node.ChunkEmbeddings = append(node.ChunkEmbeddings, make([]float32, 1024))
		}
		_, err = tenant.CreateNode(node)
		require.NoError(b, err)
	}
	require.NoError(b, async.Flush())

	b.Run("embedding_free", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := tenant.BatchGetNodesWithoutEmbeddings(ids); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := tenant.BatchGetNodes(ids); err != nil {
				b.Fatal(err)
			}
		}
	})
}
