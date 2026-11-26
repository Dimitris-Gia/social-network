// Owner: A
// GET /api/users, /api/users/{id}, PATCH privacy

package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"social-network/pkg/middleware"
	"social-network/pkg/models"
	"social-network/pkg/utils"
)

// lockedProfile is what a non-follower gets for a private profile: only what
// the page needs to show a name and a Follow button (API.md 1.2). It's a
// separate struct, not the full User with fields blanked, so email, birthday,
// about_me and counts are not in the response at all, not even as "".
type lockedProfile struct {
	models.UserSummary
	IsPrivate    bool   `json:"is_private"`
	FollowStatus string `json:"follow_status"`
	CanView      bool   `json:"can_view"`
}

type privacyRequest struct {
	// A pointer, so a missing field (nil) is told apart from false.
	IsPrivate *bool `json:"is_private"`
}

// userPathID reads {id} (or another path parameter) as a user ID and answers
// 400 itself when it isn't a positive number.
func userPathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		utils.WriteError(w, http.StatusBadRequest, "invalid user id")
		return 0, false
	}
	return id, true
}

// fillFollowCounts sets followers_count / following_count on u.
func (h *Handler) fillFollowCounts(u *models.User) error {
	var err error
	u.FollowersCount, u.FollowingCount, err = models.FollowCounts(h.DB, u.ID)
	return err
}

// ListUsers handles GET /api/users: everyone except you, for "find people".
// Only UserSummary fields are listed, so it's safe to show private users too.
//
//	curl -b cookies.txt localhost:8080/api/users
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	viewerID, _ := middleware.UserID(r)
	users, err := models.ListUsers(h.DB, viewerID)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, users)
}

// GetUser handles GET /api/users/{id}.
//
// Always 200 for an existing user. Whether you get the full profile is
// decided HERE on the server: the frontend can't be trusted to hide fields,
// because anyone can call the API with curl or read DevTools.
//
//	curl -b cookies.txt localhost:8080/api/users/3
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	viewerID, _ := middleware.UserID(r)
	targetID, ok := userPathID(w, r, "id")
	if !ok {
		return
	}

	user, err := models.GetUserByID(h.DB, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		utils.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}

	status, err := models.GetFollowStatus(h.DB, viewerID, targetID)
	if err != nil {
		utils.ServerError(w, err)
		return
	}

	// The gate, same rule as models.CanViewProfile. We already have the user
	// and the status loaded, so we apply it here without a second query.
	canView := status == models.FollowSelf || !user.IsPrivate || status == models.FollowFollowing
	if !canView {
		utils.WriteJSON(w, http.StatusOK, lockedProfile{
			UserSummary: models.UserSummary{
				ID: user.ID, FirstName: user.FirstName, LastName: user.LastName,
				Nickname: user.Nickname, Avatar: user.Avatar,
			},
			IsPrivate:    true,
			FollowStatus: status,
			CanView:      false,
		})
		return
	}

	if err := h.fillFollowCounts(user); err != nil {
		utils.ServerError(w, err)
		return
	}
	user.FollowStatus = status
	user.CanView = true
	utils.WriteJSON(w, http.StatusOK, user)
}

// SetPrivacy handles PATCH /api/me/privacy with {"is_private": true|false}.
// There's no user ID in the URL: you can only ever change YOUR OWN privacy,
// because the ID comes from the session.
//
//	curl -b cookies.txt -X PATCH localhost:8080/api/me/privacy \
//	  -H 'Content-Type: application/json' -d '{"is_private":true}'
func (h *Handler) SetPrivacy(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)

	var req privacyRequest
	if err := utils.ReadJSON(w, r, &req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.IsPrivate == nil {
		utils.WriteError(w, http.StatusBadRequest, "is_private is required")
		return
	}

	if err := models.SetPrivacy(h.DB, userID, *req.IsPrivate); err != nil {
		utils.ServerError(w, err)
		return
	}
	// Going public auto-accepted every pending request, so none of their
	// notifications is open any more.
	if !*req.IsPrivate {
		h.clearNotifications(userID, models.NotifFollowRequest, 0, 0)
	}
	utils.WriteJSON(w, http.StatusOK, map[string]bool{"is_private": *req.IsPrivate})
}
