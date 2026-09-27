package storage

import (
	"path/filepath"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

func TestBadgerStartupRepairsLegacyEdgeAdjacency(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "badger")
	options := BadgerOptions{DataDir: dataDir}
	engine, err := NewBadgerEngineWithOptions(options)
	require.NoError(t, err)
	t.Cleanup(func() {
		if engine != nil {
			_ = engine.Close()
		}
	})

	startID := NodeID("repair:owner")
	endID := NodeID("repair:item")
	edgeID := EdgeID("repair:relationship")
	require.NoError(t, engine.BulkCreateNodes([]*Node{
		{ID: startID, Labels: []string{"Owner"}},
		{ID: endID, Labels: []string{"Item"}},
	}))
	require.NoError(t, engine.BulkCreateEdges([]*Edge{{
		ID: edgeID, StartNode: startID, EndNode: endID, Type: "HAS",
	}}))
	head, err := engine.loadEdgeMVCCHead(edgeID)
	require.NoError(t, err)

	// Older bulk writers persisted the edge body and head without these keys.
	require.NoError(t, engine.db.Update(func(txn *badger.Txn) error {
		outgoingKey, err := engine.mvccOutgoingAdjacencyKeyString(txn, startID, edgeID, head.Version)
		if err != nil {
			return err
		}
		incomingKey, err := engine.mvccIncomingAdjacencyKeyString(txn, endID, edgeID, head.Version)
		if err != nil {
			return err
		}
		for _, key := range [][]byte{outgoingKey, incomingKey} {
			if _, err := txn.Get(key); err != nil {
				return err
			}
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	}))
	require.NoError(t, engine.writeSchemaVersion(storageVersionPropKeyDictV2))
	require.NoError(t, engine.Close())
	engine = nil

	_, err = NewBadgerEngineWithOptions(options)
	var upgradeErr *ErrStorageUpgradeRequired
	require.ErrorAs(t, err, &upgradeErr)
	require.Equal(t, storageVersionPropKeyDictV2, upgradeErr.OnDisk)
	require.Equal(t, storageVersionCurrent, upgradeErr.Current)

	options.AllowStorageUpgrade = true
	engine, err = NewBadgerEngineWithOptions(options)
	require.NoError(t, err)
	version, err := engine.readSchemaVersion()
	require.NoError(t, err)
	require.Equal(t, storageVersionCurrent, version)
	reader, err := engine.BeginTransaction()
	require.NoError(t, err)
	require.NoError(t, reader.SetNamespace("repair"))
	t.Cleanup(func() {
		if reader.Status == TxStatusActive {
			_ = reader.Rollback()
		}
	})

	outgoing, err := reader.GetOutgoingEdges(startID)
	require.NoError(t, err)
	require.Len(t, outgoing, 1)
	require.Equal(t, edgeID, outgoing[0].ID)
	incoming, err := reader.GetIncomingEdges(endID)
	require.NoError(t, err)
	require.Len(t, incoming, 1)
	require.Equal(t, edgeID, incoming[0].ID)
}

func TestV3MigrationPreservesRetainedLegacyEdgeHistory(t *testing.T) {
	options := BadgerOptions{
		DataDir:       filepath.Join(t.TempDir(), "badger"),
		EngineOptions: EngineOptions{RetentionPolicy: RetentionPolicy{MaxVersionsPerKey: 100}},
	}
	engine, err := NewBadgerEngineWithOptions(options)
	require.NoError(t, err)
	t.Cleanup(func() {
		if engine != nil {
			_ = engine.Close()
		}
	})
	startID, endID := NodeID("repair:a"), NodeID("repair:b")
	edgeID := EdgeID("repair:history")
	require.NoError(t, engine.BulkCreateNodes([]*Node{{ID: startID}, {ID: endID}}))
	require.NoError(t, engine.BulkCreateEdges([]*Edge{{ID: edgeID, StartNode: startID, EndNode: endID, Type: "HAS", Properties: map[string]any{"n": int64(1)}}}))
	created, err := engine.loadEdgeMVCCHead(edgeID)
	require.NoError(t, err)
	require.NoError(t, engine.db.Update(func(txn *badger.Txn) error {
		out, err := engine.mvccOutgoingAdjacencyKeyString(txn, startID, edgeID, created.Version)
		if err != nil {
			return err
		}
		in, err := engine.mvccIncomingAdjacencyKeyString(txn, endID, edgeID, created.Version)
		if err != nil {
			return err
		}
		if err := txn.Delete(out); err != nil {
			return err
		}
		return txn.Delete(in)
	}))
	require.NoError(t, engine.UpdateEdge(&Edge{ID: edgeID, StartNode: startID, EndNode: endID, Type: "HAS", Properties: map[string]any{"n": int64(2)}}))
	require.NoError(t, engine.writeSchemaVersion(storageVersionPropKeyDictV2))
	require.NoError(t, engine.Close())
	engine = nil
	options.AllowStorageUpgrade = true
	engine, err = NewBadgerEngineWithOptions(options)
	require.NoError(t, err)
	historical, err := engine.GetOutgoingEdgesVisibleAt(startID, created.Version)
	require.NoError(t, err)
	require.Len(t, historical, 1)
	require.Equal(t, int64(1), historical[0].Properties["n"])
}

func TestV3MigrationPreservesDeletedLegacyEdgeHistory(t *testing.T) {
	options := BadgerOptions{
		DataDir:       filepath.Join(t.TempDir(), "badger"),
		EngineOptions: EngineOptions{RetentionPolicy: RetentionPolicy{MaxVersionsPerKey: 100}},
	}
	engine, err := NewBadgerEngineWithOptions(options)
	require.NoError(t, err)
	t.Cleanup(func() {
		if engine != nil {
			_ = engine.Close()
		}
	})
	startID, endID := NodeID("repair:from"), NodeID("repair:to")
	edgeID := EdgeID("repair:deleted")
	require.NoError(t, engine.BulkCreateNodes([]*Node{{ID: startID}, {ID: endID}}))
	require.NoError(t, engine.BulkCreateEdges([]*Edge{{ID: edgeID, StartNode: startID, EndNode: endID, Type: "HAS"}}))
	created, err := engine.loadEdgeMVCCHead(edgeID)
	require.NoError(t, err)
	require.NoError(t, engine.db.Update(func(txn *badger.Txn) error {
		out, err := engine.mvccOutgoingAdjacencyKeyString(txn, startID, edgeID, created.Version)
		if err != nil {
			return err
		}
		in, err := engine.mvccIncomingAdjacencyKeyString(txn, endID, edgeID, created.Version)
		if err != nil {
			return err
		}
		if err := txn.Delete(out); err != nil {
			return err
		}
		return txn.Delete(in)
	}))
	require.NoError(t, engine.DeleteEdge(edgeID))
	require.NoError(t, engine.writeSchemaVersion(storageVersionPropKeyDictV2))
	require.NoError(t, engine.Close())
	engine = nil
	options.AllowStorageUpgrade = true
	engine, err = NewBadgerEngineWithOptions(options)
	require.NoError(t, err)
	historical, err := engine.GetOutgoingEdgesVisibleAt(startID, created.Version)
	require.NoError(t, err)
	require.Len(t, historical, 1)
	require.Equal(t, edgeID, historical[0].ID)
	current, err := engine.GetOutgoingEdges(startID)
	require.NoError(t, err)
	require.Empty(t, current)
}
