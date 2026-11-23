// Package middleware wraps handlers with checks that run before them.
package middleware

import (
	"context"
	"database/sql"
	"net/http"
)

// TEMPORARY VERSION (Mon 23 → Wed 25).
// It pretends every request comes from user 1, so B and C can build
// their endpoints before login exists. On Wednesday, A replaces the BODY
// of RequireAuth (cookie → sessions table → expiry check → 401).
// The two function signatures below stay the same, so nobody else's code changes.

// ctxKey is unexported, so no other package can overwrite our context value
// by accident with a plain string key.
type ctxKey struct{}

// RequireAuth runs `next` only for logged-in users and stores their ID in
// the request context.
func RequireAuth(db *sql.DB, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const fakeUserID int64 = 1 // TODO(A, Wed): read the session cookie instead
		ctx := context.WithValue(r.Context(), ctxKey{}, fakeUserID)
		next(w, r.WithContext(ctx))
	})
}

// UserID returns the logged-in user's ID. Handlers call this and must never
// trust a user ID sent in the request body or query string.
func UserID(r *http.Request) (int64, bool) {
	id, ok := r.Context().Value(ctxKey{}).(int64)
	return id, ok
}
