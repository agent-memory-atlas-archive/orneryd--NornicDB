package storage

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGH448_StagedEmbeddingWritebackKeepsLabelScanComplete reproduces issue #448:
// auto-commit label scans and count() intermittently missed committed nodes while
// the embed worker was writing embeddings back to them. The failure window is the
// staged state: UpdateNodeEmbedding parks the node in nodeCache without maintaining
// the cache-side label index, so the engine-side label index row is shadowed by the
// cache ("has a pending version") while the cache-side index never sees the node.
// Whole in-flight batches (multiples of the worker batch size) vanish from scans
// until the flush completes.
func TestGH448_StagedEmbeddingWritebackKeepsLabelScanComplete(t *testing.T) {
	badger, err := NewBadgerEngineInMemory()
	require.NoError(t, err)
	defer badger.Close()

	config := DefaultAsyncEngineConfig()
	config.FlushInterval = time.Hour // deterministic: only explicit flushes
	async := NewAsyncEngine(NewNamespacedEngine(badger, "gh448"), config)
	defer async.Close()

	const total = 64 // two embed-worker batches of 32
	const batch = 32

	ids := make([]NodeID, 0, total)
	for index := 0; index < total; index++ {
		node := &Node{
			ID:         NodeID(fmt.Sprintf("doc-%03d", index)),
			Labels:     []string{"Doc"},
			Properties: map[string]any{"seq": int64(index)},
		}
		_, err := async.CreateNode(node)
		require.NoError(t, err)
		ids = append(ids, node.ID)
	}
	require.NoError(t, async.Flush())

	sortedIDs := func(nodes []*Node) []NodeID {
		out := make([]NodeID, len(nodes))
		for i, n := range nodes {
			out[i] = n.ID
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}

	baselineNodes, err := async.GetNodesByLabel("Doc")
	require.NoError(t, err)
	baseline := sortedIDs(baselineNodes)
	require.Len(t, baseline, total)

	// Simulate the embed worker staging one in-flight batch of write-backs.
	for index := 0; index < batch; index++ {
		node, err := async.GetNode(ids[index])
		require.NoError(t, err)
		node.ChunkEmbeddings = [][]float32{{float32(index), 0.5, 1.0}}
		node.EmbedMeta = map[string]any{"embedded": true}
		require.NoError(t, async.UpdateNodeEmbedding(node))
	}

	t.Run("label scan complete while batch staged", func(t *testing.T) {
		nodes, err := async.GetNodesByLabel("Doc")
		require.NoError(t, err)
		require.Equal(t, baseline, sortedIDs(nodes))
	})

	t.Run("label count complete while batch staged", func(t *testing.T) {
		count, err := async.NodeCountByLabel("Doc")
		require.NoError(t, err)
		require.Equal(t, int64(total), count)
	})

	t.Run("projected label scan complete while batch staged", func(t *testing.T) {
		seen := make(map[NodeID]struct{})
		err := async.StreamNodesByLabelProjected("Doc", []string{"seq"}, func(node *Node) error {
			seen[node.ID] = struct{}{}
			return nil
		})
		require.NoError(t, err)
		require.Len(t, seen, total)
	})

	t.Run("id iteration complete while batch staged", func(t *testing.T) {
		seen := make(map[NodeID]struct{})
		err := async.ForEachNodeIDByLabel("Doc", func(id NodeID) bool {
			seen[id] = struct{}{}
			return true
		})
		require.NoError(t, err)
		require.Len(t, seen, total)
	})

	t.Run("staged embeddings visible pre-flush", func(t *testing.T) {
		nodes, err := async.GetNodesByLabel("Doc")
		require.NoError(t, err)
		embedded := 0
		for _, node := range nodes {
			if len(node.ChunkEmbeddings) > 0 && len(node.EmbedMeta) > 0 {
				embedded++
			}
		}
		require.Equal(t, batch, embedded)
	})

	t.Run("flush preserves completeness and embeddings", func(t *testing.T) {
		require.NoError(t, async.Flush())
		nodes, err := async.GetNodesByLabel("Doc")
		require.NoError(t, err)
		require.Equal(t, baseline, sortedIDs(nodes))
		embedded := 0
		for _, node := range nodes {
			if len(node.ChunkEmbeddings) > 0 && len(node.EmbedMeta) > 0 {
				embedded++
			}
		}
		require.Equal(t, batch, embedded)
	})
}
