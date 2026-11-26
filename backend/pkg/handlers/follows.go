// Owner: A
// follow / unfollow / accept / decline, follower lists
//
// Every handler takes "who am I" from the session (middleware.UserID), never
// from the request body. The other user's ID comes from the URL.

package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"social-network/pkg/middleware"
	"social-network/pkg/models"
	"social-network/pkg/utils"
)

// Follow handles POST /api/users/{id}/follow.
// Public profile -> {"follow_status":"following"} at once.
// Private profile -> {"follow_status":"pending"}, waits for accept.
//
//	curl -b cookies.txt -X POST localhost:8080/api/users/3/follow
func (h *Handler) Follow(w http.ResponseWriter, r *http.Request) {
	me, _ := middleware.UserID(r)
	targetID, ok := userPathID(w, r, "id")
	if !ok {
		return
	}
	if targetID == me {
		utils.WriteError(w, http.StatusBadRequest, "you can't follow yourself")
		return
	}

	status, err := models.Follow(h.DB, me, targetID)
	switch {
	case errors.Is(err, models.ErrUserNotFound):
		utils.WriteError(w, http.StatusNotFound, "user not found")
		return
	case errors.Is(err, models.ErrAlreadyFollowing):
		utils.WriteError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		utils.ServerError(w, err)
		return
	}

	// A private profile gets a request it can Accept / Decline from the bell.
	if status == models.FollowPending {
		h.notify(models.NewNotification{UserID: targetID, ActorID: me, Type: models.NotifFollowRequest})
	}
	utils.WriteJSON(w, http.StatusOK, map[string]string{"follow_status": status})
}

// Unfollow handles DELETE /api/users/{id}/follow: unfollow, or cancel your
// own pending request.
//
//	curl -b cookies.txt -X DELETE localhost:8080/api/users/3/follow
func (h *Handler) Unfollow(w http.ResponseWriter, r *http.Request) {
	me, _ := middleware.UserID(r)
	targetID, ok := userPathID(w, r, "id")
	if !ok {
		return
	}

	err := models.Unfollow(h.DB, me, targetID)
	if errors.Is(err, models.ErrNotFollowing) {
		utils.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	// If this was a pending request being cancelled, its notification must go
	// too, or the target could still "accept" it from the bell. For an
	// accepted follow there's nothing to delete, so this is a no-op.
	h.clearNotifications(targetID, models.NotifFollowRequest, me, 0)
	w.WriteHeader(http.StatusNoContent)
}

// FollowRequests handles GET /api/follow-requests: pending requests TO me.
func (h *Handler) FollowRequests(w http.ResponseWriter, r *http.Request) {
	me, _ := middleware.UserID(r)
	list, err := models.PendingFollowRequests(h.DB, me)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, list)
}

// AcceptFollow handles POST /api/follow-requests/{userId}/accept.
// {userId} is the REQUESTER. The request must be addressed to me: the model
// query has "following_id = me" in its WHERE clause, so guessing someone
// else's request just gives 404.
//
//	curl -b cookies.txt -X POST localhost:8080/api/follow-requests/5/accept
func (h *Handler) AcceptFollow(w http.ResponseWriter, r *http.Request) {
	h.answerFollowRequest(w, r, models.AcceptFollowRequest)
}

// DeclineFollow handles POST /api/follow-requests/{userId}/decline.
func (h *Handler) DeclineFollow(w http.ResponseWriter, r *http.Request) {
	h.answerFollowRequest(w, r, models.DeclineFollowRequest)
}

// answerFollowRequest is the shared body of accept and decline: they only
// differ in which model function runs.
func (h *Handler) answerFollowRequest(w http.ResponseWriter, r *http.Request,
	answer func(db *sql.DB, userID, requesterID int64) error,
) {
	me, _ := middleware.UserID(r)
	requesterID, ok := userPathID(w, r, "userId")
	if !ok {
		return
	}

	err := answer(h.DB, me, requesterID)
	if errors.Is(err, models.ErrNoFollowRequest) {
		utils.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	// The request is answered, so its Accept / Decline buttons disappear.
	h.clearNotifications(me, models.NotifFollowRequest, requesterID, 0)
	w.WriteHeader(http.StatusNoContent)
}

// Followers handles GET /api/users/{id}/followers (gated).
func (h *Handler) Followers(w http.ResponseWriter, r *http.Request) {
	h.followList(w, r, models.Followers)
}

// Following handles GET /api/users/{id}/following (gated).
func (h *Handler) Following(w http.ResponseWriter, r *http.Request) {
	h.followList(w, r, models.Following)
}

// followList applies the private-profile gate, then runs the list query.
// The audit checks that a non-follower can't see a private user's lists.
func (h *Handler) followList(w http.ResponseWriter, r *http.Request,
	list func(db *sql.DB, userID int64) ([]models.UserSummary, error),
) {
	me, _ := middleware.UserID(r)
	targetID, ok := userPathID(w, r, "id")
	if !ok {
		return
	}

	canView, err := models.CanViewProfile(h.DB, me, targetID)
	switch {
	case errors.Is(err, models.ErrUserNotFound):
		utils.WriteError(w, http.StatusNotFound, "user not found")
		return
	case err != nil:
		utils.ServerError(w, err)
		return
	case !canView:
		utils.WriteError(w, http.StatusForbidden, "this account is private")
		return
	}

	users, err := list(h.DB, targetID)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, users)
}
