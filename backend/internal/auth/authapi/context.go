package authapi

import (
	"context"
	"errors"
)

type ctxKey int

const identityCtxKey ctxKey = iota

// ErrNoIdentity is returned by IdentityFromContext when no identity is present
// (typically because a request bypassed RequireAuth middleware).
var ErrNoIdentity = errors.New("authapi: no identity in context")

// WithIdentity returns a context carrying the given identity. Called by
// RequireAuth middleware; other code should never call this directly.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityCtxKey, id)
}

// IdentityFromContext retrieves the identity attached by RequireAuth.
// Returns ErrNoIdentity if none — indicates a routing bug (protected handler
// mounted without RequireAuth middleware).
func IdentityFromContext(ctx context.Context) (*Identity, error) {
	v, ok := ctx.Value(identityCtxKey).(*Identity)
	if !ok || v == nil {
		return nil, ErrNoIdentity
	}
	return v, nil
}
