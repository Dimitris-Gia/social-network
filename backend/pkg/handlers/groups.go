// Owner: C
// Groups, invitations, join requests, and group membership handlers.

package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"

	"social-network/pkg/middleware"
	"social-network/pkg/models"
	"social-network/pkg/utils"
)

type createGroupRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type inviteRequest struct {
	UserID int64 `json:"user_id"`
}

func groupIDFromRequest(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func memberIDFromRequest(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("user_id"), 10, 64)
}

// ListGroups returns every group and the current user's status.
func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r)
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	groups, err := models.ListGroups(h.DB, userID)
	if err != nil {
		log.Printf("ListGroups: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not load groups")
		return
	}
	utils.WriteJSON(w, http.StatusOK, groups)
}

// CreateGroup creates a group and automatically makes the creator a member.
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r)
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req createGroupRequest
	if err := utils.ReadJSON(w, r, &req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Title == "" || req.Description == "" {
		utils.WriteError(w, http.StatusBadRequest, "title and description are required")
		return
	}

	groupID, err := models.CreateGroup(h.DB, userID, req.Title, req.Description)
	if err != nil {
		log.Printf("CreateGroup: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not create group")
		return
	}
	group, err := models.GetGroupForViewer(h.DB, groupID, userID)
	if err != nil {
		log.Printf("CreateGroup get group: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not load created group")
		return
	}
	utils.WriteJSON(w, http.StatusCreated, group)
}

// GetGroup returns a group to any authenticated user.
func (h *Handler) GetGroup(w http.ResponseWriter, r *http.Request) {
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

	group, err := models.GetGroupForViewer(h.DB, groupID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		utils.WriteError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		log.Printf("GetGroup: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not load group")
		return
	}
	utils.WriteJSON(w, http.StatusOK, group)
}

// RequestJoin creates a join request. It cannot be called again after the
// user is already invited, has requested, or is already a member.
func (h *Handler) RequestJoin(w http.ResponseWriter, r *http.Request) {
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
	if _, err := models.GetGroup(h.DB, groupID); errors.Is(err, sql.ErrNoRows) {
		utils.WriteError(w, http.StatusNotFound, "group not found")
		return
	} else if err != nil {
		log.Printf("RequestJoin get group: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not load group")
		return
	}

	if err := models.RequestJoin(h.DB, groupID, userID); errors.Is(err, models.ErrGroupConflict) {
		utils.WriteError(w, http.StatusConflict, "a request, invitation, or membership already exists")
		return
	} else if err != nil {
		log.Printf("RequestJoin: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not create join request")
		return
	}
	utils.WriteJSON(w, http.StatusCreated, map[string]string{"status": "requested"})
}

// InviteToGroup allows any confirmed member to invite one of their followers.
func (h *Handler) InviteToGroup(w http.ResponseWriter, r *http.Request) {
	inviterID, ok := middleware.UserID(r)
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	groupID, err := groupIDFromRequest(r)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	var req inviteRequest
	if err := utils.ReadJSON(w, r, &req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.UserID <= 0 {
		utils.WriteError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	isMember, err := models.IsMember(h.DB, inviterID, groupID)
	if err != nil {
		log.Printf("InviteToGroup membership: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not check membership")
		return
	}
	if !isMember {
		utils.WriteError(w, http.StatusForbidden, "only group members can invite users")
		return
	}

	// In follows, follower_id is the user who follows following_id.
	// Therefore req.UserID must follow the inviter.
	isFollower, err := models.IsFollower(h.DB, req.UserID, inviterID)
	if err != nil {
		log.Printf("InviteToGroup follower check: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not check follower status")
		return
	}
	if !isFollower {
		utils.WriteError(w, http.StatusForbidden, "you can only invite one of your followers")
		return
	}

	if err := models.Invite(h.DB, groupID, inviterID, req.UserID); errors.Is(err, models.ErrGroupConflict) {
		utils.WriteError(w, http.StatusConflict, "a request, invitation, or membership already exists")
		return
	} else if err != nil {
		log.Printf("InviteToGroup: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not invite user")
		return
	}
	utils.WriteJSON(w, http.StatusCreated, map[string]string{"status": "invited"})
}

// AcceptMember accepts an invitation or a join request with the correct authority.
func (h *Handler) AcceptMember(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserID(r)
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	groupID, err := groupIDFromRequest(r)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	memberID, err := memberIDFromRequest(r)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid member id")
		return
	}

	status, err := models.MembershipStatus(h.DB, groupID, memberID)
	if err != nil {
		log.Printf("AcceptMember status: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not check membership")
		return
	}

	switch status {
	case "invited":
		if actorID != memberID {
			utils.WriteError(w, http.StatusForbidden, "only the invited user can accept")
			return
		}
	case "requested":
		creator, err := models.IsCreator(h.DB, groupID, actorID)
		if err != nil {
			log.Printf("AcceptMember creator check: %v", err)
			utils.WriteError(w, http.StatusInternalServerError, "could not check group creator")
			return
		}
		if !creator {
			utils.WriteError(w, http.StatusForbidden, "only the group creator can accept join requests")
			return
		}
	default:
		utils.WriteError(w, http.StatusConflict, "no pending invitation or join request")
		return
	}

	if err := models.AcceptMember(h.DB, groupID, memberID); errors.Is(err, sql.ErrNoRows) {
		utils.WriteError(w, http.StatusNotFound, "pending membership not found")
		return
	} else if err != nil {
		log.Printf("AcceptMember: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not accept membership")
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]string{"status": "member"})
}

// DeclineMember removes an invitation or join request with the correct authority.
func (h *Handler) DeclineMember(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserID(r)
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	groupID, err := groupIDFromRequest(r)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	memberID, err := memberIDFromRequest(r)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid member id")
		return
	}

	status, err := models.MembershipStatus(h.DB, groupID, memberID)
	if err != nil {
		log.Printf("DeclineMember status: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not check membership")
		return
	}

	switch status {
	case "invited":
		if actorID != memberID {
			utils.WriteError(w, http.StatusForbidden, "only the invited user can decline")
			return
		}
	case "requested":
		creator, err := models.IsCreator(h.DB, groupID, actorID)
		if err != nil {
			log.Printf("DeclineMember creator check: %v", err)
			utils.WriteError(w, http.StatusInternalServerError, "could not check group creator")
			return
		}
		if !creator {
			utils.WriteError(w, http.StatusForbidden, "only the group creator can decline join requests")
			return
		}
	default:
		utils.WriteError(w, http.StatusConflict, "no pending invitation or join request")
		return
	}

	if err := models.DeclineMember(h.DB, groupID, memberID); errors.Is(err, sql.ErrNoRows) {
		utils.WriteError(w, http.StatusNotFound, "pending membership not found")
		return
	} else if err != nil {
		log.Printf("DeclineMember: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not decline membership")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Members returns pending and confirmed users only to confirmed members.
func (h *Handler) Members(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("Members membership: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not check membership")
		return
	}
	if !isMember {
		utils.WriteError(w, http.StatusForbidden, "only group members can view members")
		return
	}

	members, err := models.Members(h.DB, groupID)
	if err != nil {
		log.Printf("Members: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "could not load members")
		return
	}
	utils.WriteJSON(w, http.StatusOK, members)
}
