package cypher

import (
	"context"
	"fmt"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

// TestApocPathSpanningTreeBasic tests basic spanning tree functionality
func TestApocPathSpanningTreeBasic(t *testing.T) {
	baseStore := newTestMemoryEngine(t)

	store := storage.NewNamespacedEngine(baseStore, "test")
	e := NewStorageExecutor(store)
	ctx := context.Background()

	// Create a simple graph: A -> B -> C
	//                         A -> D
	store.CreateNode(&storage.Node{ID: "a", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "A"}})
	store.CreateNode(&storage.Node{ID: "b", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "B"}})
	store.CreateNode(&storage.Node{ID: "c", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "C"}})
	store.CreateNode(&storage.Node{ID: "d", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "D"}})

	store.CreateEdge(&storage.Edge{ID: "e1", Type: "CONNECTS", StartNode: "a", EndNode: "b"})
	store.CreateEdge(&storage.Edge{ID: "e2", Type: "CONNECTS", StartNode: "b", EndNode: "c"})
	store.CreateEdge(&storage.Edge{ID: "e3", Type: "CONNECTS", StartNode: "a", EndNode: "d"})

	// Get spanning tree from node A (using node ID directly)
	query := `CALL apoc.path.spanningTree({id: 'a'}, {}) YIELD path RETURN path`

	result, err := e.Execute(ctx, query, nil)
	if err != nil {
		t.Fatalf("Spanning tree query failed: %v", err)
	}

	// Should have 3 edges in the spanning tree (connecting 4 nodes)
	if len(result.Rows) != 3 {
		t.Errorf("Expected 3 edges in spanning tree, got %d", len(result.Rows))
	}
}

// TestApocPathSpanningTreeWithCycle tests spanning tree with cycles
func TestApocPathSpanningTreeWithCycle(t *testing.T) {
	baseStore := newTestMemoryEngine(t)

	store := storage.NewNamespacedEngine(baseStore, "test")
	e := NewStorageExecutor(store)
	ctx := context.Background()

	// Create a graph with a cycle: A -> B -> C -> A
	//                                A -> D
	store.CreateNode(&storage.Node{ID: "a", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "A"}})
	store.CreateNode(&storage.Node{ID: "b", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "B"}})
	store.CreateNode(&storage.Node{ID: "c", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "C"}})
	store.CreateNode(&storage.Node{ID: "d", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "D"}})

	store.CreateEdge(&storage.Edge{ID: "e1", Type: "CONNECTS", StartNode: "a", EndNode: "b"})
	store.CreateEdge(&storage.Edge{ID: "e2", Type: "CONNECTS", StartNode: "b", EndNode: "c"})
	store.CreateEdge(&storage.Edge{ID: "e3", Type: "CONNECTS", StartNode: "c", EndNode: "a"}) // Creates cycle
	store.CreateEdge(&storage.Edge{ID: "e4", Type: "CONNECTS", StartNode: "a", EndNode: "d"})

	query := `CALL apoc.path.spanningTree({id: 'a'}, {}) YIELD path RETURN path`

	result, err := e.Execute(ctx, query, nil)
	if err != nil {
		t.Fatalf("Spanning tree query failed: %v", err)
	}

	// Should still have only 3 edges (spanning tree excludes cycle-creating edge)
	if len(result.Rows) != 3 {
		t.Errorf("Expected 3 edges in spanning tree (no cycles), got %d", len(result.Rows))
	}
}

// TestApocPathSpanningTreeMaxLevel tests maxLevel configuration
func TestApocPathSpanningTreeMaxLevel(t *testing.T) {
	baseStore := newTestMemoryEngine(t)

	store := storage.NewNamespacedEngine(baseStore, "test")
	e := NewStorageExecutor(store)
	ctx := context.Background()

	// Create a chain: A -> B -> C -> D
	store.CreateNode(&storage.Node{ID: "a", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "A"}})
	store.CreateNode(&storage.Node{ID: "b", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "B"}})
	store.CreateNode(&storage.Node{ID: "c", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "C"}})
	store.CreateNode(&storage.Node{ID: "d", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "D"}})

	store.CreateEdge(&storage.Edge{ID: "e1", Type: "CONNECTS", StartNode: "a", EndNode: "b"})
	store.CreateEdge(&storage.Edge{ID: "e2", Type: "CONNECTS", StartNode: "b", EndNode: "c"})
	store.CreateEdge(&storage.Edge{ID: "e3", Type: "CONNECTS", StartNode: "c", EndNode: "d"})

	// Test with maxLevel: 2 (should get only first 2 edges)
	query := `CALL apoc.path.spanningTree({id: 'a'}, {maxLevel: 2}) YIELD path RETURN path`

	result, err := e.Execute(ctx, query, nil)
	if err != nil {
		t.Fatalf("Spanning tree query failed: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Errorf("Expected 2 edges with maxLevel:2, got %d", len(result.Rows))
	}
}

// TestApocPathSpanningTreeRelationshipFilter tests relationship type filtering
func TestApocPathSpanningTreeRelationshipFilter(t *testing.T) {
	baseStore := newTestMemoryEngine(t)

	store := storage.NewNamespacedEngine(baseStore, "test")
	e := NewStorageExecutor(store)
	ctx := context.Background()

	// Create a graph with different relationship types
	store.CreateNode(&storage.Node{ID: "a", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "A"}})
	store.CreateNode(&storage.Node{ID: "b", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "B"}})
	store.CreateNode(&storage.Node{ID: "c", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "C"}})
	store.CreateNode(&storage.Node{ID: "d", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "D"}})

	store.CreateEdge(&storage.Edge{ID: "e1", Type: "FRIEND", StartNode: "a", EndNode: "b"})
	store.CreateEdge(&storage.Edge{ID: "e2", Type: "FRIEND", StartNode: "b", EndNode: "c"})
	store.CreateEdge(&storage.Edge{ID: "e3", Type: "COLLEAGUE", StartNode: "a", EndNode: "d"})

	// Test with relationshipFilter for FRIEND only
	query := `CALL apoc.path.spanningTree({id: 'a'}, {relationshipFilter: 'FRIEND'}) YIELD path RETURN path`

	result, err := e.Execute(ctx, query, nil)
	if err != nil {
		t.Fatalf("Spanning tree query failed: %v", err)
	}

	// Should have only 2 edges (FRIEND relationships)
	if len(result.Rows) != 2 {
		t.Errorf("Expected 2 edges with FRIEND filter, got %d", len(result.Rows))
	}
}

// TestApocPathSpanningTreeDFS tests depth-first search spanning tree
func TestApocPathSpanningTreeDFS(t *testing.T) {
	baseStore := newTestMemoryEngine(t)

	store := storage.NewNamespacedEngine(baseStore, "test")
	e := NewStorageExecutor(store)
	ctx := context.Background()

	// Create a binary tree
	store.CreateNode(&storage.Node{ID: "a", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "A"}})
	store.CreateNode(&storage.Node{ID: "b", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "B"}})
	store.CreateNode(&storage.Node{ID: "c", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "C"}})
	store.CreateNode(&storage.Node{ID: "d", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "D"}})
	store.CreateNode(&storage.Node{ID: "e", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "E"}})

	store.CreateEdge(&storage.Edge{ID: "e1", Type: "CONNECTS", StartNode: "a", EndNode: "b"})
	store.CreateEdge(&storage.Edge{ID: "e2", Type: "CONNECTS", StartNode: "a", EndNode: "c"})
	store.CreateEdge(&storage.Edge{ID: "e3", Type: "CONNECTS", StartNode: "b", EndNode: "d"})
	store.CreateEdge(&storage.Edge{ID: "e4", Type: "CONNECTS", StartNode: "b", EndNode: "e"})

	// Test with DFS
	query := `CALL apoc.path.spanningTree({id: 'a'}, {bfs: false}) YIELD path RETURN path`

	result, err := e.Execute(ctx, query, nil)
	if err != nil {
		t.Fatalf("Spanning tree DFS query failed: %v", err)
	}

	// Should have 4 edges in the spanning tree
	if len(result.Rows) != 4 {
		t.Errorf("Expected 4 edges in DFS spanning tree, got %d", len(result.Rows))
	}
}

// TestApocPathSpanningTreeLabelFilter tests label filtering
func TestApocPathSpanningTreeLabelFilter(t *testing.T) {
	baseStore := newTestMemoryEngine(t)

	store := storage.NewNamespacedEngine(baseStore, "test")
	e := NewStorageExecutor(store)
	ctx := context.Background()

	// Create a graph with different labels
	store.CreateNode(&storage.Node{ID: "a", Labels: []string{"Start"}, Properties: map[string]interface{}{"name": "A"}})
	store.CreateNode(&storage.Node{ID: "b", Labels: []string{"Good"}, Properties: map[string]interface{}{"name": "B"}})
	store.CreateNode(&storage.Node{ID: "c", Labels: []string{"Good"}, Properties: map[string]interface{}{"name": "C"}})
	store.CreateNode(&storage.Node{ID: "d", Labels: []string{"Bad"}, Properties: map[string]interface{}{"name": "D"}})

	store.CreateEdge(&storage.Edge{ID: "e1", Type: "CONNECTS", StartNode: "a", EndNode: "b"})
	store.CreateEdge(&storage.Edge{ID: "e2", Type: "CONNECTS", StartNode: "b", EndNode: "c"})
	store.CreateEdge(&storage.Edge{ID: "e3", Type: "CONNECTS", StartNode: "a", EndNode: "d"})

	// Test with labelFilter to include only Good nodes
	query := `CALL apoc.path.spanningTree({id: 'a'}, {labelFilter: '+Good'}) YIELD path RETURN path`

	result, err := e.Execute(ctx, query, nil)
	if err != nil {
		t.Fatalf("Spanning tree query failed: %v", err)
	}

	// Should have only 2 edges (excluding Bad labeled node)
	if len(result.Rows) != 2 {
		t.Errorf("Expected 2 edges with Good label filter, got %d", len(result.Rows))
	}
}

// TestApocPathSpanningTreeLimit tests limit configuration
func TestApocPathSpanningTreeLimit(t *testing.T) {
	baseStore := newTestMemoryEngine(t)

	store := storage.NewNamespacedEngine(baseStore, "test")
	e := NewStorageExecutor(store)
	ctx := context.Background()

	// Create a star graph: A connected to B, C, D, E
	store.CreateNode(&storage.Node{ID: "a", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "A"}})
	store.CreateNode(&storage.Node{ID: "b", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "B"}})
	store.CreateNode(&storage.Node{ID: "c", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "C"}})
	store.CreateNode(&storage.Node{ID: "d", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "D"}})
	store.CreateNode(&storage.Node{ID: "e", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "E"}})

	store.CreateEdge(&storage.Edge{ID: "e1", Type: "CONNECTS", StartNode: "a", EndNode: "b"})
	store.CreateEdge(&storage.Edge{ID: "e2", Type: "CONNECTS", StartNode: "a", EndNode: "c"})
	store.CreateEdge(&storage.Edge{ID: "e3", Type: "CONNECTS", StartNode: "a", EndNode: "d"})
	store.CreateEdge(&storage.Edge{ID: "e4", Type: "CONNECTS", StartNode: "a", EndNode: "e"})

	// Test with limit: 2
	query := `CALL apoc.path.spanningTree({id: 'a'}, {limit: 2}) YIELD path RETURN path`

	result, err := e.Execute(ctx, query, nil)
	if err != nil {
		t.Fatalf("Spanning tree query failed: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Errorf("Expected 2 edges with limit:2, got %d", len(result.Rows))
	}
}

// TestApocPathSpanningTreeDisconnectedGraph tests with disconnected components
func TestApocPathSpanningTreeDisconnectedGraph(t *testing.T) {
	baseStore := newTestMemoryEngine(t)

	store := storage.NewNamespacedEngine(baseStore, "test")
	e := NewStorageExecutor(store)
	ctx := context.Background()

	// Create two disconnected components: A-B and C-D
	store.CreateNode(&storage.Node{ID: "a", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "A"}})
	store.CreateNode(&storage.Node{ID: "b", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "B"}})
	store.CreateNode(&storage.Node{ID: "c", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "C"}})
	store.CreateNode(&storage.Node{ID: "d", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "D"}})

	store.CreateEdge(&storage.Edge{ID: "e1", Type: "CONNECTS", StartNode: "a", EndNode: "b"})
	store.CreateEdge(&storage.Edge{ID: "e2", Type: "CONNECTS", StartNode: "c", EndNode: "d"})

	// Get spanning tree from node A (should only include A-B component)
	query := `CALL apoc.path.spanningTree({id: 'a'}, {}) YIELD path RETURN path`

	result, err := e.Execute(ctx, query, nil)
	if err != nil {
		t.Fatalf("Spanning tree query failed: %v", err)
	}

	// Should have only 1 edge (A-B), not reaching C-D
	if len(result.Rows) != 1 {
		t.Errorf("Expected 1 edge in disconnected graph spanning tree, got %d", len(result.Rows))
	}
}

// TestApocPathSpanningTreeDirection tests directional traversal
func TestApocPathSpanningTreeDirection(t *testing.T) {
	baseStore := newTestMemoryEngine(t)

	store := storage.NewNamespacedEngine(baseStore, "test")
	e := NewStorageExecutor(store)
	ctx := context.Background()

	// Create a directed graph: A -> B -> C
	//                           A <- D
	store.CreateNode(&storage.Node{ID: "a", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "A"}})
	store.CreateNode(&storage.Node{ID: "b", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "B"}})
	store.CreateNode(&storage.Node{ID: "c", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "C"}})
	store.CreateNode(&storage.Node{ID: "d", Labels: []string{"Node"}, Properties: map[string]interface{}{"name": "D"}})

	store.CreateEdge(&storage.Edge{ID: "e1", Type: "POINTS_TO", StartNode: "a", EndNode: "b"})
	store.CreateEdge(&storage.Edge{ID: "e2", Type: "POINTS_TO", StartNode: "b", EndNode: "c"})
	store.CreateEdge(&storage.Edge{ID: "e3", Type: "POINTS_TO", StartNode: "d", EndNode: "a"})

	// Test with outgoing relationships only
	query := `CALL apoc.path.spanningTree({id: 'a'}, {relationshipFilter: '>POINTS_TO'}) YIELD path RETURN path`

	result, err := e.Execute(ctx, query, nil)
	if err != nil {
		t.Fatalf("Spanning tree query failed: %v", err)
	}

	// Should have only 2 edges (A->B->C), not including D->A
	if len(result.Rows) != 2 {
		t.Errorf("Expected 2 edges with outgoing filter, got %d", len(result.Rows))
	}
}

// TestSpanningTreeKernel_BFSAndDFSShareFrontierInvariants pins the shared
// frontier walk (spanningTreeFrom): on a tree graph both modes visit every
// node and return the same edge set; on a cyclic graph both return a
// connected spanning forest (nodeCount - 1 edges for the reachable part).
func TestSpanningTreeKernel_BFSAndDFSShareFrontierInvariants(t *testing.T) {
	baseStore := newTestMemoryEngine(t)
	store := storage.NewNamespacedEngine(baseStore, "spanning_kernel")
	e := NewStorageExecutor(store)

	for i := 0; i < 6; i++ {
		_, err := store.CreateNode(&storage.Node{ID: storage.NodeID(fmt.Sprintf("n%d", i)), Labels: []string{"Node"}})
		require.NoError(t, err)
	}
	// A tree: n0 -> n1, n0 -> n2, n1 -> n3, n1 -> n4, n2 -> n5
	treeEdges := []*storage.Edge{
		{ID: "t0", Type: "LINK", StartNode: "n0", EndNode: "n1"},
		{ID: "t1", Type: "LINK", StartNode: "n0", EndNode: "n2"},
		{ID: "t2", Type: "LINK", StartNode: "n1", EndNode: "n3"},
		{ID: "t3", Type: "LINK", StartNode: "n1", EndNode: "n4"},
		{ID: "t4", Type: "LINK", StartNode: "n2", EndNode: "n5"},
	}
	for _, edge := range treeEdges {
		require.NoError(t, store.CreateEdge(edge))
	}

	start, err := store.GetNode("n0")
	require.NoError(t, err)

	bfs := e.spanningTreeFrom(start, apocPathConfig{direction: "both", maxLevel: -1}, false)
	dfs := e.spanningTreeFrom(start, apocPathConfig{direction: "both", maxLevel: -1}, true)
	require.Len(t, bfs, 5)
	require.Len(t, dfs, 5)
	require.ElementsMatch(t, []storage.EdgeID{"t0", "t1", "t2", "t3", "t4"},
		[]storage.EdgeID{bfs[0].ID, bfs[1].ID, bfs[2].ID, bfs[3].ID, bfs[4].ID})
	require.ElementsMatch(t, []storage.EdgeID{"t0", "t1", "t2", "t3", "t4"},
		[]storage.EdgeID{dfs[0].ID, dfs[1].ID, dfs[2].ID, dfs[3].ID, dfs[4].ID})

	// A cycle: n1 -extra-> n2. Both walks still span all reachable nodes
	// with exactly nodeCount-1 edges.
	require.NoError(t, store.CreateEdge(&storage.Edge{ID: "cycle", Type: "LINK", StartNode: "n1", EndNode: "n2"}))
	bfs = e.spanningTreeFrom(start, apocPathConfig{direction: "both", maxLevel: -1}, false)
	dfs = e.spanningTreeFrom(start, apocPathConfig{direction: "both", maxLevel: -1}, true)
	require.Len(t, bfs, 5)
	require.Len(t, dfs, 5)
}

// BenchmarkSpanningTreeKernel pins the shared frontier-walk cost on a 100-node
// synthetic tree for both modes.
func BenchmarkSpanningTreeKernel(b *testing.B) {
	baseStore := newTestMemoryEngine(b)
	store := storage.NewNamespacedEngine(baseStore, "spanning_bench")
	e := NewStorageExecutor(store)
	for i := 0; i < 100; i++ {
		if _, err := store.CreateNode(&storage.Node{ID: storage.NodeID(fmt.Sprintf("n%d", i)), Labels: []string{"Node"}}); err != nil {
			b.Fatal(err)
		}
		if i > 0 {
			parent := storage.NodeID(fmt.Sprintf("n%d", (i-1)/2))
			child := storage.NodeID(fmt.Sprintf("n%d", i))
			if err := store.CreateEdge(&storage.Edge{ID: storage.EdgeID(fmt.Sprintf("e%d", i)), Type: "LINK", StartNode: parent, EndNode: child}); err != nil {
				b.Fatal(err)
			}
		}
	}
	start, err := store.GetNode("n0")
	if err != nil {
		b.Fatal(err)
	}
	cfg := apocPathConfig{direction: "both", maxLevel: -1}
	b.Run("bfs", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = e.spanningTreeFrom(start, cfg, false)
		}
	})
	b.Run("dfs", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = e.spanningTreeFrom(start, cfg, true)
		}
	})
}
