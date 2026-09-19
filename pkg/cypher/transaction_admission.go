package cypher

import "github.com/orneryd/nornicdb/pkg/storage"

// beginTransactionSnapshot opens a transaction immediately after the async
// cache visibility barrier. The async flush lock is held only across the flush
// and snapshot creation, never across query execution or transaction lifetime.
func beginTransactionSnapshot(asyncEngine *storage.AsyncEngine, txEngine TransactionCapableEngine) (*storage.BadgerTransaction, error) {
	var tx *storage.BadgerTransaction
	openSnapshot := func() error {
		var err error
		tx, err = txEngine.BeginTransaction()
		return err
	}
	if asyncEngine != nil {
		if err := asyncEngine.FlushBeforeSnapshot(openSnapshot); err != nil {
			return nil, err
		}
	} else if err := openSnapshot(); err != nil {
		return nil, err
	}
	return tx, nil
}
