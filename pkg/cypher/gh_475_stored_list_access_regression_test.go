package cypher

import (
	"context"
	"testing"

	"github.com/orneryd/nornicdb/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestGH475_StoredListSubscriptAndHeadInMatch(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		mode := "autocommit"
		if explicit {
			mode = "explicit transaction"
		}
		t.Run(mode, func(t *testing.T) {
			store := storage.NewNamespacedEngine(newTestMemoryEngine(t), "gh475")
			for index := int64(1); index <= 4; index++ {
				tag := "t0"
				if index%2 == 1 {
					tag = "t1"
				}
				_, err := store.CreateNode(&storage.Node{
					ID: storage.NodeID(string(rune('0' + index))), Labels: []string{"A"},
					Properties: map[string]interface{}{"id": index, "name": "n" + string(rune('0'+index)), "tags": []string{tag, "all"}},
				})
				require.NoError(t, err)
			}
			exec := NewStorageExecutor(store)
			ctx := context.Background()
			if explicit {
				_, err := exec.Execute(ctx, "BEGIN", nil)
				require.NoError(t, err)
			}

			projected, err := exec.Execute(ctx, "MATCH (a:A) RETURN a.id AS id, a.tags[0] AS first ORDER BY id LIMIT 2", nil)
			require.NoError(t, err)
			require.Equal(t, [][]interface{}{{int64(1), "t1"}, {int64(2), "t0"}}, projected.Rows)

			filtered, err := exec.Execute(ctx, "MATCH (a:A) WHERE a.tags[0] = 't0' RETURN a.id AS id ORDER BY id", nil)
			require.NoError(t, err)
			require.Equal(t, [][]interface{}{{int64(2)}, {int64(4)}}, filtered.Rows)

			headed, err := exec.Execute(ctx, "MATCH (a:A) WHERE head(a.tags) = 't0' RETURN count(a) AS c", nil)
			require.NoError(t, err)
			require.Equal(t, [][]interface{}{{int64(2)}}, headed.Rows)

			if explicit {
				_, err := exec.Execute(ctx, "COMMIT", nil)
				require.NoError(t, err)
			}
		})
	}
}
