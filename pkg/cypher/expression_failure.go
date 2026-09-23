package cypher

import (
	"context"
	"sync"
)

type expressionFailureKey struct{}

type expressionFailure struct {
	mu  sync.Mutex
	err error
}

func recordExpressionFailure(ctx context.Context, err error) {
	if failure, ok := ctx.Value(expressionFailureKey{}).(*expressionFailure); ok {
		failure.mu.Lock()
		if failure.err == nil {
			failure.err = err
		}
		failure.mu.Unlock()
	}
}

func getExpressionFailure(ctx context.Context) error {
	if failure, ok := ctx.Value(expressionFailureKey{}).(*expressionFailure); ok {
		failure.mu.Lock()
		defer failure.mu.Unlock()
		return failure.err
	}
	return nil
}
