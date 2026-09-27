package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newPendingIndexBenchEngine(b *testing.B) (*AsyncEngine, *SchemaManager) {
	b.Helper()
	badger, err := NewBadgerEngineInMemory()
	require.NoError(b, err)
	b.Cleanup(func() { _ = badger.Close() })
	config := DefaultAsyncEngineConfig()
	config.FlushInterval = time.Hour
	async := NewAsyncEngine(badger, config)
	b.Cleanup(func() { _ = async.Close() })
	schema := async.GetSchemaForNamespace("nornic")
	require.NoError(b, schema.AddPropertyIndex("bench_id", "Bench", []string{"id"}))
	return async, schema
}

// BenchmarkAsyncEngine_CreateNode is the async write path, which keeps the
// pending-write view (#719) in step with the write cache.
func BenchmarkAsyncEngine_CreateNode(b *testing.B) {
	async, _ := newPendingIndexBenchEngine(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i > 0 && i%10000 == 0 {
			b.StopTimer()
			require.NoError(b, async.Flush())
			b.StartTimer()
		}
		_, err := async.CreateNode(&Node{
			ID:     NodeID(fmt.Sprintf("nornic:bench-%d", i)),
			Labels: []string{"Bench"},
			Properties: map[string]any{
				"id": fmt.Sprintf("id-%d", i), "seq": int64(i), "name": "bench", "score": 1.5, "active": true,
			},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAsyncEngine_BulkDeleteEdges(b *testing.B) {
	for _, size := range []int{1, 16} {
		b.Run(fmt.Sprintf("ids=%d", size), func(b *testing.B) {
			badger, err := NewBadgerEngineInMemory()
			require.NoError(b, err)
			config := DefaultAsyncEngineConfig()
			config.FlushInterval = time.Hour
			async := NewAsyncEngine(badger, config)
			b.Cleanup(func() {
				_ = async.Close()
				_ = badger.Close()
			})
			ids := make([]EdgeID, size)
			for index := range ids {
				ids[index] = EdgeID(fmt.Sprintf("bench:e-%d", index))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if err := async.BulkDeleteEdges(ids); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkAsyncPropertyIndexLookup is a property-index seek on the async
// stack with nothing pending and with a backlog of pending writes (#719).
func BenchmarkAsyncPropertyIndexLookup(b *testing.B) {
	for _, pending := range []int{0, 1000} {
		b.Run(fmt.Sprintf("pending=%d", pending), func(b *testing.B) {
			async, schema := newPendingIndexBenchEngine(b)
			const stored = 10000
			for i := 0; i < stored; i++ {
				_, err := async.CreateNode(&Node{ID: NodeID(fmt.Sprintf("nornic:s-%d", i)), Labels: []string{"Bench"}, Properties: map[string]any{"id": fmt.Sprintf("id-%d", i)}})
				require.NoError(b, err)
			}
			require.NoError(b, async.Flush())
			for i := 0; i < pending; i++ {
				_, err := async.CreateNode(&Node{ID: NodeID(fmt.Sprintf("nornic:p-%d", i)), Labels: []string{"Bench"}, Properties: map[string]any{"id": fmt.Sprintf("pending-%d", i)}})
				require.NoError(b, err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if ids := schema.PropertyIndexLookup("Bench", "id", fmt.Sprintf("id-%d", i%stored)); len(ids) != 1 {
					b.Fatalf("lookup %d: %v", i, ids)
				}
			}
		})
	}
}
