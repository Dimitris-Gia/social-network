// Package handlers is the "app" layer: it decodes requests, checks
// permissions, calls pkg/models, and answers with JSON. No SQL here.
package handlers

import (
	"database/sql"
	"net/http"

	"social-network/pkg/middleware"
	"social-network/pkg/utils"
)

// Handler carries what every handler needs. Each owner adds methods on it
// in their own file (auth.go, posts.go, groups.go...), so nobody needs
// global variables and nobody edits someone else's file.
//
// B adds `Hub *websocket.Hub` here on Mon 30 Nov.
type Handler struct {
	DB *sql.DB
}

// RegisterRoutes lists EVERY API route in one place.
// Reading this function answers the audit question "which routes need a login?":
// anything registered with protected(...) goes through RequireAuth first.
//
// Patterns use Go 1.22+ ServeMux syntax ("METHOD /path/{param}"), standard
// library only. Read a path parameter with r.PathValue("id").
// A wrong method (e.g. GET on a POST route) gets an automatic 405.
func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	h := &Handler{DB: db}

	// protected registers a route that requires a valid session.
	protected := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.RequireAuth(db, fn))
	}

	// ── Public (no login) ────────────────────────────────────
	mux.HandleFunc("GET /api/health", h.Health)
	// A (Tue/Wed):
	mux.HandleFunc("POST /api/register", h.Register)
	// mux.HandleFunc("POST /api/login", h.Login)

	// ── A: accounts & social graph ───────────────────────────
	protected("GET /api/whoami", h.WhoAmI) // temporary test route, delete once /api/me exists
	// protected("POST /api/logout", h.Logout)
	// protected("GET /api/me", h.Me)
	// protected("GET /api/users", h.ListUsers)
	// protected("GET /api/users/{id}", h.GetUser)
	// protected("PATCH /api/users/me/privacy", h.SetPrivacy)
	// protected("POST /api/users/{id}/follow", h.Follow)
	// protected("DELETE /api/users/{id}/follow", h.Unfollow)
	// protected("POST /api/follow-requests/{id}/accept", h.AcceptFollow)
	// protected("POST /api/follow-requests/{id}/decline", h.DeclineFollow)
	// protected("GET /api/users/{id}/followers", h.Followers)
	// protected("GET /api/users/{id}/following", h.Following)
	// protected("GET /api/notifications", h.ListNotifications)
	// protected("GET /api/notifications/unread-count", h.UnreadNotifications)
	// protected("POST /api/notifications/{id}/read", h.MarkNotificationRead)

	// ── B: posts, comments, chat ─────────────────────────────
	// protected("POST /api/posts", h.CreatePost)
	// protected("GET /api/posts/feed", h.Feed)
	// protected("GET /api/posts/{id}", h.GetPost)
	// protected("GET /api/users/{id}/posts", h.UserPosts)
	// protected("GET /api/posts/{id}/comments", h.ListComments)
	// protected("POST /api/posts/{id}/comments", h.CreateComment)
	// protected("GET /ws", h.ServeWS)

	// ── C: groups & events ───────────────────────────────────
	protected("GET /api/groups", h.ListGroups)
	protected("POST /api/groups", h.CreateGroup)
	protected("GET /api/groups/{id}", h.GetGroup)
	protected("POST /api/groups/{id}/invite", h.InviteToGroup)
	protected("POST /api/groups/{id}/join", h.RequestJoin)
	protected("GET /api/groups/{id}/members", h.Members)
	protected("POST /api/groups/{id}/members/{user_id}/accept", h.AcceptMember)
	protected("POST /api/groups/{id}/members/{user_id}/decline", h.DeclineMember)
	protected("GET /api/groups/{id}/events", h.ListEvents)
	protected("POST /api/groups/{id}/events", h.CreateEvent)
	protected("POST /api/groups/{id}/events/{event_id}/response", h.RespondEvent)

	// Any other /api/... path: answer in JSON, not Go's plain-text 404,
	// so the React client can always do res.json().
	// Side effect: a WRONG METHOD on a real route (POST on a GET route) also
	// lands here instead of Go's automatic 405, hence the hint in the message.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		utils.WriteError(w, http.StatusNotFound, "not found (check the URL and the HTTP method)")
	})
}

// Health lets you (and Docker later) check the server is up and the DB answers.
//
//	curl -i localhost:8080/api/health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if err := h.DB.PingContext(r.Context()); err != nil {
		utils.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// WhoAmI proves the middleware puts the user ID in the request context.
// Delete it when GET /api/me exists (Wed).
func (h *Handler) WhoAmI(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	utils.WriteJSON(w, http.StatusOK, map[string]int64{"userId": userID})
}
