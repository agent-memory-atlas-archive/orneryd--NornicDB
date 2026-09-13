package search

import (
	"context"
	"errors"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestSearchTextChunksUsesOuterRRFAndBM25Fallback(t *testing.T) {
	t.Run("fuses independently searched chunks", func(t *testing.T) {
		var searchedQueries []string
		response, err := SearchTextChunks(
			context.Background(),
			"complete query",
			&SearchOptions{Limit: 3},
			func(context.Context, string) ([]string, error) {
				return []string{"chunk one", "chunk two", "chunk three"}, nil
			},
			func(_ context.Context, chunk string) ([]float32, error) {
				switch chunk {
				case "chunk one":
					return []float32{1, 0}, nil
				case "chunk two":
					return []float32{0, 1}, nil
				case "chunk three":
					return []float32{-1, 0}, nil
				default:
					return nil, errors.New("full query must not be embedded")
				}
			},
			func(_ context.Context, query string, embedding []float32, opts *SearchOptions) (*SearchResponse, error) {
				searchedQueries = append(searchedQueries, query)
				require.NotNil(t, embedding)
				require.Equal(t, 10, opts.Limit)
				switch query {
				case "chunk one":
					return &SearchResponse{Results: []SearchResult{
						{NodeID: storage.NodeID("single"), Score: 0.99},
						{NodeID: storage.NodeID("repeated"), Score: 0.20, Properties: map[string]any{"representative_score": 0.20}},
					}}, nil
				case "chunk two":
					return &SearchResponse{Results: []SearchResult{
						{NodeID: storage.NodeID("other"), Score: 0.80},
						{NodeID: storage.NodeID("repeated"), Score: 0.70, Properties: map[string]any{"representative_score": 0.70}},
					}}, nil
				case "chunk three":
					return &SearchResponse{Results: []SearchResult{
						{NodeID: storage.NodeID("tie-b"), Score: 0.50},
						{NodeID: storage.NodeID("tie-a"), Score: 0.40},
					}}, nil
				default:
					t.Fatalf("unexpected search query %q", query)
					return nil, nil
				}
			},
		)
		require.NoError(t, err)
		require.Equal(t, []string{"chunk one", "chunk two", "chunk three"}, searchedQueries)
		require.Equal(t, "chunked_rrf_hybrid", response.SearchMethod)
		require.Equal(t, storage.NodeID("repeated"), response.Results[0].NodeID)
		require.Equal(t, 0.70, response.Results[0].Properties["representative_score"])
		require.InDelta(t, 2.0/62.0, response.Results[0].Score, 0.0000001)
	})

	t.Run("falls back once when no vector chunk succeeds", func(t *testing.T) {
		var searches int
		response, err := SearchTextChunks(
			context.Background(),
			"complete query",
			&SearchOptions{Limit: 2},
			func(context.Context, string) ([]string, error) {
				return []string{"chunk one", "chunk two"}, nil
			},
			func(context.Context, string) ([]float32, error) {
				return nil, errors.New("embedding unavailable")
			},
			func(_ context.Context, query string, embedding []float32, opts *SearchOptions) (*SearchResponse, error) {
				searches++
				require.Equal(t, "complete query", query)
				require.Nil(t, embedding)
				require.Equal(t, 2, opts.Limit)
				return &SearchResponse{SearchMethod: "bm25"}, nil
			},
		)
		require.NoError(t, err)
		require.Equal(t, 1, searches)
		require.Equal(t, "bm25", response.SearchMethod)
	})

	t.Run("honors disabled fallback after empty vector results", func(t *testing.T) {
		fallbackEnabled := false
		searches := 0
		response, err := SearchTextChunks(
			context.Background(),
			"complete query",
			&SearchOptions{Limit: 2, FallbackEnabled: &fallbackEnabled},
			func(context.Context, string) ([]string, error) {
				return []string{"chunk one", "chunk two"}, nil
			},
			func(context.Context, string) ([]float32, error) {
				return []float32{1, 0}, nil
			},
			func(_ context.Context, _ string, embedding []float32, _ *SearchOptions) (*SearchResponse, error) {
				searches++
				require.NotNil(t, embedding, "disabled fallback must not issue a BM25 search")
				return &SearchResponse{SearchMethod: "rrf_hybrid"}, nil
			},
		)
		require.NoError(t, err)
		require.Equal(t, 2, searches)
		require.Equal(t, "chunked_rrf_hybrid", response.SearchMethod)
		require.Empty(t, response.Results)
	})
}
