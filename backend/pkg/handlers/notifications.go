// Owner: A
// list, unread count, mark read

package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"social-network/pkg/middleware"
	"social-network/pkg/models"
	"social-network/pkg/utils"
	"social-network/pkg/websocket"
)

// notify creates a notification and pushes it live to the receiver's open
// tabs. Everyone (A, B, C) creates notifications through this, e.g. C:
//
//	h.notify(models.NewNotification{UserID: inviteeID, ActorID: me,
//		Type: models.NotifGroupInvite, GroupID: groupID})
//
// A failure is only logged, never returned: the real action (the follow, the
// invite) already succeeded, and a missing notification must not turn that
// into an error for the user.
func (h *Handler) notify(n models.NewNotification) {
	id, err := models.CreateNotification(h.DB, n)
	if err != nil {
		log.Printf("notify %s to user %d: %v", n.Type, n.UserID, err)
		return
	}
	if h.Hub == nil {
		return
	}
	full, err := models.GetNotification(h.DB, id)
	if err != nil {
		log.Printf("notify: load %d: %v", id, err)
		return
	}
	// Same envelope as chat frames (API.md 1.4): {"type":"notification","data":{...}}.
	// An offline user simply gets it from GET /api/notifications next time.
	h.Hub.SendToUser(n.UserID, websocket.Message{Type: "notification", Data: full})
}

// clearNotifications deletes a request's notifications once it's answered or
// cancelled (see models.DeleteNotifications). Like notify, it only logs.
func (h *Handler) clearNotifications(userID int64, typ string, actorID, groupID int64) {
	if err := models.DeleteNotifications(h.DB, userID, typ, actorID, groupID); err != nil {
		log.Printf("clear %s for user %d: %v", typ, userID, err)
	}
}

// ListNotifications handles GET /api/notifications: my 50 newest.
//
//	curl -b cookies.txt localhost:8080/api/notifications
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	me, _ := middleware.UserID(r)
	list, err := models.ListNotifications(h.DB, me)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, list)
}

// UnreadNotifications handles GET /api/notifications/unread-count.
// This is the bell's badge, separate from B's unread-messages badge.
func (h *Handler) UnreadNotifications(w http.ResponseWriter, r *http.Request) {
	me, _ := middleware.UserID(r)
	n, err := models.UnreadNotificationCount(h.DB, me)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]int{"count": n})
}

// MarkNotificationRead handles POST /api/notifications/{id}/read.
// 404 both when the ID doesn't exist and when it belongs to someone else.
func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	me, _ := middleware.UserID(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		utils.WriteError(w, http.StatusBadRequest, "invalid notification id")
		return
	}

	err = models.MarkNotificationRead(h.DB, id, me)
	if errors.Is(err, models.ErrNotificationNotFound) {
		utils.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarkAllNotificationsRead handles POST /api/notifications/read-all
// (called when the bell dropdown opens).
func (h *Handler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	me, _ := middleware.UserID(r)
	if err := models.MarkAllNotificationsRead(h.DB, me); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
