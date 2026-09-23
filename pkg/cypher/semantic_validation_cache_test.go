package cypher

import (
	"context"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestSemanticValidationCachesOnlySuccessfulQueries(t *testing.T) {
	exec := NewStorageExecutor(storage.NewNamespacedEngine(newTestMemoryEngine(t), "semantic_cache"))
	validQuery := "RETURN 1 AS value"

	_, err := exec.Execute(context.Background(), validQuery, nil)
	require.NoError(t, err)
	require.True(t, exec.semanticValidationCache.contains(validQuery))

	invalidQuery := "RETURN 1 AS value, 2 AS value"
	_, err = exec.Execute(context.Background(), invalidQuery, nil)
	require.Error(t, err)
	require.False(t, exec.semanticValidationCache.contains(invalidQuery))
}
