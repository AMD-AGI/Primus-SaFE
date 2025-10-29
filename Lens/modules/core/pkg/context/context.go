package context

import (
	"context"
	"sync"
)

type contextKey struct{}

var (
	builtInContextKey       = contextKey{}
	builtInContextKeyLocker sync.Mutex
	contextCopyIgnoreKey    = new(sync.Map)
)

func findOrCreateContextMap(ctx context.Context) (*sync.Map, context.Context) {
	ctxMap, ok := ctx.Value(builtInContextKey).(*sync.Map)

	builtInContextKeyLocker.Lock()
	if !ok {
		ctxMap, ok = ctx.Value(builtInContextKey).(*sync.Map)
		if !ok {
			ctxMap = new(sync.Map)
			ctx = context.WithValue(ctx, builtInContextKey, ctxMap)
		}
	}
	builtInContextKeyLocker.Unlock()

	return ctxMap, ctx
}

func findContextMap(ctx context.Context) (*sync.Map, bool) {
	ctxMap, ok := ctx.Value(builtInContextKey).(*sync.Map)
	return ctxMap, ok
}

func WithObject(ctx context.Context, key string, obj interface{}) context.Context {
	ctxMap, ctx := findOrCreateContextMap(ctx)
	ctxMap.Store(key, obj)
	return ctx
}

func WithoutObject(ctx context.Context, key string) context.Context {
	ctxMap, ctx := findOrCreateContextMap(ctx)
	ctxMap.Delete(key)
	return ctx
}

// GetValue returns key/value in context
func GetValue(ctx context.Context, key string) (interface{}, bool) {
	if ctxMap, ok := findContextMap(ctx); ok {
		val, ok := ctxMap.Load(key)
		return val, ok
	}
	return nil, false
}

// ShallowCopyCtx returns a copied context from an exist context, without transaction and trace
func ShallowCopyCtx(ctx context.Context) context.Context {
	newCtx := context.Background()
	if ctxMap, ok := findContextMap(ctx); ok {
		newCtxMap := new(sync.Map)
		ctxMap.Range(func(key, value interface{}) bool {
			// ignore transaction
			if _, ok := contextCopyIgnoreKey.Load(key); !ok {
				newCtxMap.Store(key, value)
			}
			return true
		})
		newCtx = context.WithValue(newCtx, builtInContextKey, newCtxMap)
	}
	return newCtx
}
