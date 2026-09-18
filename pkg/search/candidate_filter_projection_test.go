package search

import (
	"context"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

type candidateFilterReadSpy struct {
	storage.Engine

	lightNodes          map[storage.NodeID]*storage.Node
	fullBatchReads      int
	fullIndividualReads int
	lightBatchReads     int
	lightReads          int
	batchErr            error
}

func (s *candidateFilterReadSpy) BatchGetNodes(ids []storage.NodeID) (map[storage.NodeID]*storage.Node, error) {
	s.fullBatchReads++
	return s.Engine.BatchGetNodes(ids)
}

func (s *candidateFilterReadSpy) GetNode(id storage.NodeID) (*storage.Node, error) {
	s.fullIndividualReads++
	return s.Engine.GetNode(id)
}

func (s *candidateFilterReadSpy) BatchGetNodesWithoutEmbeddings(ids []storage.NodeID) (map[storage.NodeID]*storage.Node, error) {
	s.lightBatchReads++
	if s.batchErr != nil {
		return nil, s.batchErr
	}
	result := make(map[storage.NodeID]*storage.Node, len(ids))
	for _, id := range ids {
		if node, ok := s.lightNodes[id]; ok {
			result[id] = node
		}
	}
	return result, nil
}

func (s *candidateFilterReadSpy) GetNodeWithoutEmbeddings(id storage.NodeID) (*storage.Node, error) {
	s.lightReads++
	if node, ok := s.lightNodes[id]; ok {
		return node, nil
	}
	return nil, storage.ErrNotFound
}

func TestCandidateFiltersUseEmbeddingFreeBatchReads(t *testing.T) {
	base := storage.NewMemoryEngine()
	t.Cleanup(func() { require.NoError(t, base.Close()) })
	spy := &candidateFilterReadSpy{
		Engine: base,
		lightNodes: map[storage.NodeID]*storage.Node{
			"doc-a": {ID: "doc-a", Labels: []string{"Document"}, Properties: map[string]any{"state": "ready"}},
			"doc-b": {ID: "doc-b", Labels: []string{"Document"}, Properties: map[string]any{"state": "draft"}},
		},
	}
	service := NewService(spy)

	got := service.filterByTypeAndProperties(
		context.Background(),
		[]indexResult{{ID: "doc-a"}, {ID: "doc-b"}},
		[]string{"Document"},
		map[string][]string{"state": {"ready"}},
		map[string]bool{},
	)

	require.Equal(t, []indexResult{{ID: "doc-a"}}, got)
	require.Equal(t, 1, spy.lightBatchReads)
	require.Zero(t, spy.fullBatchReads, "candidate filtering must not decode stored embeddings")
	require.Zero(t, spy.fullIndividualReads, "candidate filtering must not decode stored embeddings")
	require.Zero(t, spy.lightReads, "supported batch readers must not degrade to per-node reads")
}
