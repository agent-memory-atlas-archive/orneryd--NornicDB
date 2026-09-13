package cypher

import (
	"context"
	"fmt"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
)

func newClauseSemanticsBenchmarkExecutor(b *testing.B) (*StorageExecutor, storage.Engine) {
	b.Helper()
	base := newTestMemoryEngine(b)
	store := storage.NewNamespacedEngine(base, "clause-semantics-bench")
	exec := NewStorageExecutor(store)
	exec.cache = nil
	return exec, store
}

func createBenchmarkNode(b *testing.B, store storage.Engine, id, label string) storage.NodeID {
	b.Helper()
	nodeID, err := store.CreateNode(&storage.Node{
		ID:     storage.NodeID(id),
		Labels: []string{label},
		Properties: map[string]interface{}{
			"id": id,
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	return nodeID
}

func createBenchmarkEdge(b *testing.B, store storage.Engine, id, edgeType string, start, end storage.NodeID) {
	b.Helper()
	if err := store.CreateEdge(&storage.Edge{
		ID:        storage.EdgeID(id),
		Type:      edgeType,
		StartNode: start,
		EndNode:   end,
	}); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkMixedPatternRelationshipNodeProduct(b *testing.B) {
	exec, store := newClauseSemanticsBenchmarkExecutor(b)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		start := createBenchmarkNode(b, store, fmt.Sprintf("mixed-source-%d", i), "BenchMixedSource")
		end := createBenchmarkNode(b, store, fmt.Sprintf("mixed-target-%d", i), "BenchMixedTarget")
		createBenchmarkEdge(b, store, fmt.Sprintf("mixed-edge-%d", i), "BENCH_MIXED", start, end)
	}
	for i := 0; i < 100; i++ {
		createBenchmarkNode(b, store, fmt.Sprintf("mixed-item-%d", i), "BenchMixedItem")
	}

	const query = `MATCH (:BenchMixedSource)-[r:BENCH_MIXED]->(:BenchMixedTarget), (x:BenchMixedItem) RETURN count(r) AS n`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := exec.Execute(ctx, query, nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Rows) != 1 || result.Rows[0][0] != int64(1000) {
			b.Fatalf("unexpected result: %v", result.Rows)
		}
	}
}

func BenchmarkMatchUnwindMatchRelationshipMerge(b *testing.B) {
	exec, store := newClauseSemanticsBenchmarkExecutor(b)
	ctx := context.Background()
	target := createBenchmarkNode(b, store, "merge-target", "BenchMergeTarget")
	childIDs := make([]string, 100)
	for i := range childIDs {
		childIDs[i] = fmt.Sprintf("merge-child-%d", i)
		child := createBenchmarkNode(b, store, childIDs[i], "BenchMergeChild")
		createBenchmarkEdge(b, store, fmt.Sprintf("merge-edge-%d", i), "BENCH_MERGED", child, target)
	}

	const query = `
		MATCH (t:BenchMergeTarget {id: 'merge-target'})
		UNWIND $childIDs AS childID
		MATCH (c:BenchMergeChild {id: childID})
		MERGE (c)-[:BENCH_MERGED]->(t)`
	params := map[string]interface{}{"childIDs": childIDs}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := exec.Execute(ctx, query, params); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReverseOptionalTwoHopProjection(b *testing.B) {
	exec, store := newClauseSemanticsBenchmarkExecutor(b)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		repository := createBenchmarkNode(b, store, fmt.Sprintf("optional-repository-%d", i), "BenchOptionalRepository")
		file := createBenchmarkNode(b, store, fmt.Sprintf("optional-file-%d", i), "BenchOptionalFile")
		function := createBenchmarkNode(b, store, fmt.Sprintf("optional-function-%d", i), "BenchOptionalFunction")
		createBenchmarkEdge(b, store, fmt.Sprintf("optional-repository-edge-%d", i), "BENCH_REPO_CONTAINS", repository, file)
		createBenchmarkEdge(b, store, fmt.Sprintf("optional-file-edge-%d", i), "BENCH_CONTAINS", file, function)
	}

	const query = `
		MATCH (e:BenchOptionalFunction)
		OPTIONAL MATCH (e)<-[:BENCH_CONTAINS]-(f:BenchOptionalFile)<-[:BENCH_REPO_CONTAINS]-(r:BenchOptionalRepository)
		RETURN f.id AS fileID, r.id AS repositoryID`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := exec.Execute(ctx, query, nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Rows) != 100 {
			b.Fatalf("unexpected row count: %d", len(result.Rows))
		}
	}
}

func BenchmarkDistinctAndNestedAggregation(b *testing.B) {
	exec, store := newClauseSemanticsBenchmarkExecutor(b)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		start := createBenchmarkNode(b, store, fmt.Sprintf("aggregate-source-%d", i), "BenchAggregateSource")
		end := createBenchmarkNode(b, store, fmt.Sprintf("aggregate-target-%d", i), "BenchAggregateTarget")
		createBenchmarkEdge(b, store, fmt.Sprintf("aggregate-edge-%d", i), "BENCH_AGGREGATE", start, end)
	}

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "distinct relationship",
			query: `MATCH (:BenchAggregateSource)-[r:BENCH_AGGREGATE]-(:BenchAggregateTarget) RETURN count(DISTINCT r) AS n`,
		},
		{
			name:  "nested distinct relationship",
			query: `MATCH (:BenchAggregateSource)-[r:BENCH_AGGREGATE]-(:BenchAggregateTarget) RETURN size(collect(DISTINCT r)) AS n`,
		},
	}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result, err := exec.Execute(ctx, test.query, nil)
				if err != nil {
					b.Fatal(err)
				}
				if len(result.Rows) != 1 || result.Rows[0][0] != int64(100) {
					b.Fatalf("unexpected result: %v", result.Rows)
				}
			}
		})
	}
}
