package search

import (
	"strings"

	"github.com/orneryd/nornicdb/pkg/storage"
)

// filterByIndexedType applies the search API's label-or-type-property semantics
// from compact metadata maintained alongside the search indexes. It deliberately
// avoids loading nodes: document properties can be orders of magnitude larger
// than the labels needed by this predicate.
func (s *Service) filterByIndexedType(results []indexResult, types []string) ([]indexResult, bool) {
	s.indexMu.RLock()
	defer s.indexMu.RUnlock()
	for _, result := range results {
		if _, known := s.nodeLabels[result.ID]; !known {
			return nil, false
		}
	}

	// Candidate slices are pipeline-owned and discarded after filtering. Reuse
	// their backing array to keep this metadata-only path allocation-free.
	filtered := results[:0]
	for _, result := range results {
		labels := s.nodeLabels[result.ID]
		matched := false
		for _, label := range labels {
			for _, nodeType := range types {
				if strings.EqualFold(label, nodeType) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			if indexedType, hasType := s.nodeTypes[result.ID]; hasType {
				for _, nodeType := range types {
					if strings.EqualFold(indexedType, nodeType) {
						matched = true
						break
					}
				}
			}
		}
		if matched {
			filtered = append(filtered, result)
		}
	}
	return filtered, true
}

func nodeMatchesType(node *storage.Node, wanted map[string]struct{}) bool {
	for _, label := range node.Labels {
		if _, ok := wanted[strings.ToLower(label)]; ok {
			return true
		}
	}
	nodeType, ok := node.Properties["type"].(string)
	if !ok {
		return false
	}
	_, ok = wanted[strings.ToLower(nodeType)]
	return ok
}
