package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BenchmarkAsyncEngine_UpdateNodeEmbedding measures the staged embedding
// write-back path (GH-448): label-index maintenance must keep scans complete
// without adding meaningful cost per write-back.
func BenchmarkAsyncEngine_UpdateNodeEmbedding(b *testing.B) {
	badger, err := NewBadgerEngineInMemory()
	require.NoError(b, err)
	defer badger.Close()

	config := DefaultAsyncEngineConfig()
	config.FlushInterval = time.Hour
	async := NewAsyncEngine(NewNamespacedEngine(badger, "gh448bench"), config)
	defer async.Close()

	// 4096 nodes across 64 distinct labels: the label-index maintenance cost
	// scales with the bucket count, so keep it representative.
	const total = 4096
	const labelBuckets = 64
	staged := make([]*Node, 0, total)
	for index := 0; index < total; index++ {
		node := &Node{
			ID:         NodeID(fmt.Sprintf("doc-%04d", index)),
			Labels:     []string{fmt.Sprintf("Label%02d", index%labelBuckets)},
			Properties: map[string]any{"seq": int64(index)},
		}
		_, err := async.CreateNode(node)
		require.NoError(b, err)
		if index%32 == 31 {
			require.NoError(b, async.Flush())
		}
	}
	require.NoError(b, async.Flush())

	// Pre-staged worker-style copies with embeddings applied.
	for index := 0; index < total; index++ {
		latest, err := async.GetNode(NodeID(fmt.Sprintf("doc-%04d", index)))
		require.NoError(b, err)
		latest.ChunkEmbeddings = [][]float32{{float32(index), 0.5, 1.0}}
		latest.EmbedMeta = map[string]any{"embedded": true}
		staged = append(staged, latest)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Copy so each op stages a fresh object like the worker does.
		fresh := CopyNode(staged[i%total])
		if err := async.UpdateNodeEmbedding(fresh); err != nil {
			b.Fatal(err)
		}
	}
}
