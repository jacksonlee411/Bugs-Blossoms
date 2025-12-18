package services

import "context"

type skipCacheInvalidationKey struct{}
type skipOutboxEnqueueKey struct{}

func WithSkipCacheInvalidation(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipCacheInvalidationKey{}, true)
}

func WithSkipOutboxEnqueue(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipOutboxEnqueueKey{}, true)
}

func shouldSkipCacheInvalidation(ctx context.Context) bool {
	v := ctx.Value(skipCacheInvalidationKey{})
	skip, _ := v.(bool)
	return skip
}

func shouldSkipOutboxEnqueue(ctx context.Context) bool {
	v := ctx.Value(skipOutboxEnqueueKey{})
	skip, _ := v.(bool)
	return skip
}
