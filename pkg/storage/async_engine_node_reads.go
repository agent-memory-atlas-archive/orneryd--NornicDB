package storage

// GetNodeWithoutEmbeddings checks pending async state before delegating an
// embedding-free read. Cached nodes are copied without vector payloads so
// callers cannot mutate write-behind state and do not pay vector copy costs.
func (ae *AsyncEngine) GetNodeWithoutEmbeddings(id NodeID) (*Node, error) {
	ae.mu.RLock()
	if ae.deleteNodes[id] {
		ae.mu.RUnlock()
		return nil, ErrNotFound
	}
	if node, ok := ae.nodeCache[id]; ok {
		result := copyNodeWithoutEmbeddings(node)
		ae.mu.RUnlock()
		return result, nil
	}
	ae.mu.RUnlock()

	if reader, ok := ae.engine.(NodeWithoutEmbeddingsReader); ok {
		return reader.GetNodeWithoutEmbeddings(id)
	}
	return ae.engine.GetNode(id)
}

// BatchGetNodesWithoutEmbeddings merges pending async nodes with a real
// batched embedding-free read from the wrapped engine. Async cache state wins
// over persisted state, matching BatchGetNodes semantics.
func (ae *AsyncEngine) BatchGetNodesWithoutEmbeddings(ids []NodeID) (map[NodeID]*Node, error) {
	if len(ids) == 0 {
		return make(map[NodeID]*Node), nil
	}
	result := make(map[NodeID]*Node, len(ids))
	missing := make([]NodeID, 0, len(ids))
	ae.mu.RLock()
	for _, id := range ids {
		if id == "" || ae.deleteNodes[id] {
			continue
		}
		if node, cached := ae.nodeCache[id]; cached {
			result[id] = copyNodeWithoutEmbeddings(node)
			continue
		}
		missing = append(missing, id)
	}
	ae.mu.RUnlock()
	if len(missing) == 0 {
		return result, nil
	}
	reader, ok := ae.engine.(BatchNodeWithoutEmbeddingsReader)
	if !ok || !batchNodeWithoutEmbeddingsSupported(ae.engine) {
		return nil, ErrNotImplemented
	}

	persisted, err := reader.BatchGetNodesWithoutEmbeddings(missing)
	if err != nil {
		return nil, err
	}

	// Reconcile writes accepted while the underlying batch was in flight.
	ae.mu.RLock()
	for _, id := range missing {
		if ae.deleteNodes[id] {
			delete(result, id)
			continue
		}
		if node, cached := ae.nodeCache[id]; cached {
			result[id] = copyNodeWithoutEmbeddings(node)
			continue
		}
		if node := persisted[id]; node != nil {
			result[id] = node
		}
	}
	ae.mu.RUnlock()
	return result, nil
}

// BatchGetNodesWithoutEmbeddingsSupported reports whether misses can remain
// batched and embedding-free through the wrapped engine stack.
func (ae *AsyncEngine) BatchGetNodesWithoutEmbeddingsSupported() bool {
	return batchNodeWithoutEmbeddingsSupported(ae.engine)
}
