package cypher

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissingIDPropertyReturnsNull(t *testing.T) {
	executor := setupTestExecutor(t)
	executeBehaviorQuery(t, executor, "CREATE (:Document {name: 'without-id'})")

	result := executeBehaviorQuery(t, executor,
		"MATCH (n:Document {name: 'without-id'}) RETURN n.id AS propertyID, id(n) AS internalID")

	require.Len(t, result.Rows, 1)
	require.Nil(t, result.Rows[0][0])
	require.NotNil(t, result.Rows[0][1])
}
