package embed

import (
	"context"
	"fmt"
)

// embedDocumentPropertyBatchFallback preserves the generic structured-batch
// contract for providers that only expose single-document structured calls.
// Wrappers use it as a compatibility fallback without hiding real provider
// batching when the wrapped provider supports it.
func embedDocumentPropertyBatchFallback(
	ctx context.Context,
	fallbackTexts []string,
	properties []map[string]any,
	maxTokens, overlap int,
	embedOne func(context.Context, string, map[string]any, int, int) (*DocumentChunkResult, error),
) ([]*DocumentChunkResult, error) {
	if len(fallbackTexts) != len(properties) {
		return nil, fmt.Errorf("structured document batch length mismatch: %d texts, %d property maps", len(fallbackTexts), len(properties))
	}
	results := make([]*DocumentChunkResult, len(fallbackTexts))
	for i := range fallbackTexts {
		result, err := embedOne(ctx, fallbackTexts[i], properties[i], maxTokens, overlap)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}
