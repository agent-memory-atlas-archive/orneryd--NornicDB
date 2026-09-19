package storage

import (
	"strings"

	"github.com/dgraph-io/badger/v4"
)

func (b *BadgerEngine) GetNodeVisibleAt(id NodeID, version MVCCVersion) (*Node, error) {
	return b.getNodeVisibleAtWithView(id, version, b.withView)
}

func (b *BadgerEngine) getNodeVisibleAtWithView(id NodeID, version MVCCVersion, view func(func(*badger.Txn) error) error) (*Node, error) {
	deregister, err := b.beginMVCCSnapshotRead(version)
	if err != nil {
		return nil, err
	}
	defer deregister()
	var node *Node
	err = view(func(txn *badger.Txn) error {
		var loadErr error
		node, loadErr = b.getNodeVisibleAtInTxn(txn, id, version)
		return loadErr
	})
	return node, err
}

// getNodeVisibleAtInTxn resolves one logical MVCC version using the supplied
// physical Badger snapshot. Callers that scan an index in the same snapshot
// use this helper to avoid opening nested views or scanning unrelated nodes.
func (b *BadgerEngine) getNodeVisibleAtInTxn(txn *badger.Txn, id NodeID, version MVCCVersion) (*Node, error) {
	head, err := b.loadNodeMVCCHeadInTxn(txn, id)
	if err != nil {
		return nil, err
	}
	if version.Compare(head.FloorVersion) < 0 {
		return nil, ErrNotVisibleAtSnapshot
	}

	if version.Compare(head.Version) >= 0 && !head.Tombstoned {
		item, getErr := txn.Get(nodeKey(id))
		if getErr == nil {
			itemVersion := item.Version()
			if cached, ok := b.cacheLoadNodeBody(id, itemVersion); ok {
				cached, loadErr := b.loadNodeEmbeddings(txn, cached, id)
				if loadErr != nil {
					return nil, loadErr
				}
				if b.filterNodeByDecay(cached, DecayScoringTime()) {
					return nil, ErrNotFound
				}
				return cached, nil
			}
			var node *Node
			if err := item.Value(func(val []byte) error {
				namespace := namespaceForNodeID(id)
				decoded, decodeErr := b.decodeNode(namespace, val)
				if decodeErr != nil {
					return decodeErr
				}
				if decoded == nil {
					return ErrNotFound
				}
				b.cacheStoreNodeBody(id, itemVersion, decoded)
				node, decodeErr = b.loadNodeEmbeddings(txn, decoded, id)
				return decodeErr
			}); err != nil {
				return nil, err
			}
			if b.filterNodeByDecay(node, DecayScoringTime()) {
				return nil, ErrNotFound
			}
			return node, nil
		}
		if getErr != badger.ErrKeyNotFound {
			return nil, getErr
		}
	}
	if head.Tombstoned && version.Compare(head.Version) >= 0 {
		return nil, ErrNotFound
	}

	var record mvccNodeRecord
	switch {
	case version.Compare(head.Version) >= 0:
		record, err = b.loadNodeMVCCRecordExactInTxn(txn, id, head.Version)
	case version.Compare(head.FloorVersion) == 0:
		record, err = b.loadNodeMVCCRecordExactInTxn(txn, id, head.FloorVersion)
	default:
		record, _, err = b.loadNodeMVCCRecordAtOrBeforeInTxn(txn, id, version)
	}
	if err != nil {
		if err == ErrNotFound && version.Compare(head.Version) >= 0 {
			record, _, err = b.loadNodeMVCCRecordAtOrBeforeInTxn(txn, id, head.Version)
		}
		if err != nil {
			return nil, err
		}
	}
	if record.Tombstoned || record.Node == nil {
		return nil, ErrNotFound
	}
	node := copyNode(record.Node)
	if b.filterNodeByDecay(node, DecayScoringTime()) {
		return nil, ErrNotFound
	}
	return node, nil
}

func (b *BadgerEngine) GetNodesByLabelVisibleAt(label string, version MVCCVersion) ([]*Node, error) {
	return b.getNodesByLabelVisibleAtWithView(label, version, b.withView)
}

func (b *BadgerEngine) getNodesByLabelVisibleAtWithView(label string, version MVCCVersion, view func(func(*badger.Txn) error) error) ([]*Node, error) {
	deregister, err := b.beginMVCCSnapshotRead(version)
	if err != nil {
		return nil, err
	}
	defer deregister()
	var nodes []*Node
	normalizedLabel := normalizeLabel(label)
	err = view(func(txn *badger.Txn) error {
		return b.iterateNodesVisibleAtInTxn(txn, version, func(node *Node) error {
			if node == nil {
				return nil
			}
			if normalizedLabel != "" {
				matched := false
				for _, existing := range node.Labels {
					if normalizeLabel(existing) == normalizedLabel {
						matched = true
						break
					}
				}
				if !matched {
					return nil
				}
			}
			nodes = append(nodes, node)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

// getNodesByLabelVisibleAtSnapshotWithView resolves label candidates from the
// same physical Badger snapshot used by an explicit transaction. The label
// index in that snapshot already represents membership at BEGIN, so work is
// proportional to the matching label rather than every stored node.
func (b *BadgerEngine) getNodesByLabelVisibleAtSnapshotWithView(label string, version MVCCVersion, view func(func(*badger.Txn) error) error) ([]*Node, error) {
	deregister, err := b.beginMVCCSnapshotRead(version)
	if err != nil {
		return nil, err
	}
	defer deregister()

	normalizedLabel := normalizeLabel(label)
	nodes := make([]*Node, 0)
	err = view(func(txn *badger.Txn) error {
		prefix := labelIndexPrefix(normalizedLabel)
		it := txn.NewIterator(badgerIterOptsKeyOnly(prefix))
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()
			nodeNum, ok := extractNodeNumIDFromLabelIndex(key, len(normalizedLabel))
			if !ok {
				continue
			}
			nodeID, ok := b.idDict.lookupNodeIDByNum(nodeNum)
			if !ok || nodeID == "" {
				continue
			}
			node, getErr := b.getNodeVisibleAtInTxn(txn, nodeID, version)
			if getErr == ErrNotFound || getErr == ErrNotVisibleAtSnapshot {
				continue
			}
			if getErr != nil {
				return getErr
			}
			matched := false
			for _, existing := range node.Labels {
				if normalizeLabel(existing) == normalizedLabel {
					matched = true
					break
				}
			}
			if matched {
				nodes = append(nodes, node)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (b *BadgerEngine) GetEdgesByTypeVisibleAt(edgeType string, version MVCCVersion) ([]*Edge, error) {
	deregister, err := b.beginMVCCSnapshotRead(version)
	if err != nil {
		return nil, err
	}
	defer deregister()
	var edges []*Edge
	normalizedType := strings.ToLower(edgeType)
	err = b.withView(func(txn *badger.Txn) error {
		return b.iterateEdgesVisibleAtInTxn(txn, version, func(edge *Edge) error {
			if edge == nil {
				return nil
			}
			if normalizedType != "" && strings.ToLower(edge.Type) != normalizedType {
				return nil
			}
			edges = append(edges, edge)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return edges, nil
}

// getEdgesByTypeVisibleAtSnapshotWithView resolves type candidates from the
// same physical Badger snapshot used by an explicit transaction. The type
// index in that snapshot represents membership at BEGIN, so unrelated edge
// bodies are never decoded.
func (b *BadgerEngine) getEdgesByTypeVisibleAtSnapshotWithView(edgeType string, version MVCCVersion, view func(func(*badger.Txn) error) error) ([]*Edge, error) {
	deregister, err := b.beginMVCCSnapshotRead(version)
	if err != nil {
		return nil, err
	}
	defer deregister()

	normalizedType := strings.ToLower(edgeType)
	edges := make([]*Edge, 0)
	err = view(func(txn *badger.Txn) error {
		if normalizedType == "" {
			return b.iterateEdgesVisibleAtInTxn(txn, version, func(edge *Edge) error {
				edges = append(edges, edge)
				return nil
			})
		}
		prefix := edgeTypeIndexPrefix(edgeType)
		it := txn.NewIterator(badgerIterOptsKeyOnly(prefix))
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			edgeNum, ok := extractEdgeNumIDFromEdgeTypeKey(it.Item().Key())
			if !ok {
				continue
			}
			edgeID, ok := b.idDict.lookupEdgeIDByNum(edgeNum)
			if !ok || edgeID == "" {
				continue
			}
			edge, getErr := b.getEdgeVisibleAtInTxn(txn, edgeID, version)
			if getErr == ErrNotFound || getErr == ErrNotVisibleAtSnapshot {
				continue
			}
			if getErr != nil {
				return getErr
			}
			if edge != nil && strings.ToLower(edge.Type) == normalizedType {
				edges = append(edges, edge)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return edges, nil
}

func (b *BadgerEngine) GetEdgesBetweenVisibleAt(startID, endID NodeID, version MVCCVersion) ([]*Edge, error) {
	if startID == "" || endID == "" {
		return nil, ErrInvalidID
	}
	deregister, err := b.beginMVCCSnapshotRead(version)
	if err != nil {
		return nil, err
	}
	defer deregister()
	var edges []*Edge
	err = b.withView(func(txn *badger.Txn) error {
		return b.iterateEdgesVisibleAtInTxn(txn, version, func(edge *Edge) error {
			if edge == nil {
				return nil
			}
			if edge.StartNode == startID && edge.EndNode == endID {
				edges = append(edges, edge)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return edges, nil
}

func (b *BadgerEngine) IterateLatestVisibleNodes(yield func(*Node) error) error {
	nodes, err := b.AllNodes()
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if err := yield(node); err != nil {
			return err
		}
	}
	return nil
}

func (b *BadgerEngine) IterateLatestVisibleEdges(yield func(*Edge) error) error {
	edges, err := b.AllEdges()
	if err != nil {
		return err
	}
	for _, edge := range edges {
		if err := yield(edge); err != nil {
			return err
		}
	}
	return nil
}
