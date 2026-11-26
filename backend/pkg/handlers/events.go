// Owner: C
// Create event, list events, and handle Going / Not going responses.

package handlers

import (
	"log"
	"net/http"
	"strconv"

	"social-network/pkg/middleware"
	"social-network/pkg/models"
	"social-network/pkg/utils"
)

type createEventRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	EventTime   string `json:"event_time"`
}

type eventResponseRequest struct {
	Response string `json:"response"`
}

// ListEvents returns a group's events to confirmed members only.
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r)
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	groupID, err := groupIDFromRequest(r)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	isMember, err := models.IsMember(h.DB, userID, groupID)
	if err != nil {
		log.Printf("ListEvents membership: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not check membership")
		return
	}
	if !isMember {
		utils.WriteError(w, http.StatusForbidden, "only group members can view events")
		return
	}

	events, err := models.EventsByGroup(h.DB, groupID, userID)
	if err != nil {
		log.Printf("ListEvents: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not load events")
		return
	}
	utils.WriteJSON(w, http.StatusOK, events)
}

// CreateEvent lets any confirmed member create an event in the group.
func (h *Handler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r)
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	groupID, err := groupIDFromRequest(r)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	isMember, err := models.IsMember(h.DB, userID, groupID)
	if err != nil {
		log.Printf("CreateEvent membership: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not check membership")
		return
	}
	if !isMember {
		utils.WriteError(w, http.StatusForbidden, "only group members can create events")
		return
	}

	var req createEventRequest
	if err := utils.ReadJSON(w, r, &req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Title == "" || req.Description == "" || req.EventTime == "" {
		utils.WriteError(w, http.StatusBadRequest, "title, description, and event_time are required")
		return
	}

	eventID, err := models.CreateEvent(h.DB, groupID, userID, req.Title, req.Description, req.EventTime)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, err := models.GetEvent(h.DB, eventID)
	if err != nil {
		log.Printf("CreateEvent get event: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not load created event")
		return
	}

	memberIDs, err := models.ConfirmedMemberIDs(h.DB, groupID)
	if err != nil {
		log.Printf("CreateEvent member notifications: %v", err)
	} else {
		for _, memberID := range memberIDs {
			if memberID == userID {
				continue
			}
			h.notify(models.NewNotification{
				UserID:  memberID,
				ActorID: userID,
				Type:    models.NotifGroupEvent,
				GroupID: groupID,
				EventID: eventID,
			})
		}
	}

	utils.WriteJSON(w, http.StatusCreated, event)
}

// RespondEvent records or changes a confirmed member's Going/Not going answer.
func (h *Handler) RespondEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r)
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	groupID, err := groupIDFromRequest(r)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	eventID, err := strconv.ParseInt(r.PathValue("event_id"), 10, 64)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	belongs, err := models.EventBelongsToGroup(h.DB, eventID, groupID)
	if err != nil {
		log.Printf("RespondEvent event check: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not check event")
		return
	}
	if !belongs {
		utils.WriteError(w, http.StatusNotFound, "event not found in this group")
		return
	}

	isMember, err := models.IsMember(h.DB, userID, groupID)
	if err != nil {
		log.Printf("RespondEvent membership: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not check membership")
		return
	}
	if !isMember {
		utils.WriteError(w, http.StatusForbidden, "only group members can respond to events")
		return
	}

	var req eventResponseRequest
	if err := utils.ReadJSON(w, r, &req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Response != "going" && req.Response != "not_going" {
		utils.WriteError(w, http.StatusBadRequest, "response must be going or not_going")
		return
	}

	if err := models.Respond(h.DB, eventID, userID, req.Response); err != nil {
		log.Printf("RespondEvent: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not save response")
		return
	}

	events, err := models.EventsByGroup(h.DB, groupID, userID)
	if err != nil {
		log.Printf("RespondEvent reload events: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "response saved but events could not be reloaded")
		return
	}
	for _, event := range events {
		if event.ID == eventID {
			utils.WriteJSON(w, http.StatusOK, event)
			return
		}
	}
	utils.WriteError(w, http.StatusNotFound, "event not found")
}
