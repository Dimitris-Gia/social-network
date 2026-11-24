// Owner: B
// GET /ws upgrade, history, "who can I chat with"

package handlers

import (
	"net/http"

	"social-network/pkg/middleware"
)

// ServeWS handles GET /ws: it upgrades the request to a WebSocket and keeps
// it open. The route is registered with protected(...), so RequireAuth has
// already checked the session cookie (no cookie: 401 and no upgrade) and the
// user ID comes from the session, never from the URL or from a frame.
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	h.Hub.ServeWS(w, r, userID)
}
