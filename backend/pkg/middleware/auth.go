// Package middleware wraps handlers with checks that run before them.
// Owner: A
package middleware

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"

	"social-network/pkg/models"
	"social-network/pkg/utils"
)

// CookieName is the name of the session cookie (API.md, Part 0).
const CookieName = "session_id"

// ctxKey is unexported, so no other package can overwrite our context value
// by accident with a plain string key.
type ctxKey struct{}

// RequireAuth runs `next` only for logged-in users and stores their ID in
// the request context. Otherwise it answers 401 and `next` never runs.
//
// Steps: read the cookie → look the ID up in the sessions table (the query
// also checks expires_at) → put the user ID in the context.
// The browser only holds a random ID; who you are is decided by the database,
// so a user can't change "who they are" by editing the cookie.
func RequireAuth(db *sql.DB, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil || cookie.Value == "" {
			utils.WriteError(w, http.StatusUnauthorized, "not logged in")
			return
		}

		userID, err := models.GetUserIDBySession(db, cookie.Value)
		if errors.Is(err, models.ErrNoSession) {
			// Unknown or expired: also tell the browser to drop the dead cookie.
			ClearSessionCookie(w)
			utils.WriteError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		if err != nil {
			log.Printf("RequireAuth: %v", err)
			utils.WriteError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		ctx := context.WithValue(r.Context(), ctxKey{}, userID)
		next(w, r.WithContext(ctx))
	})
}

// UserID returns the logged-in user's ID. Handlers call this and must never
// trust a user ID sent in the request body or query string.
func UserID(r *http.Request) (int64, bool) {
	id, ok := r.Context().Value(ctxKey{}).(int64)
	return id, ok
}

// SetSessionCookie sends the session ID to the browser.
//
//   - HttpOnly: JavaScript (including an XSS payload) can't read the cookie.
//   - SameSite=Lax: another website can't make the browser send it with a
//     POST/PATCH/DELETE, which blocks CSRF on every state-changing route.
//   - Path=/: sent with /api, /ws and /uploads.
//   - MaxAge: same 7 days as expires_at in the DB.
//   - Secure is off because dev and Docker run on plain HTTP; with HTTPS it
//     should be turned on.
func SetSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(models.SessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie tells the browser to delete the cookie right away
// (MaxAge < 0 sends "Max-Age=0"). Name, Path and flags must match the
// original cookie, or the browser treats it as a different cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
