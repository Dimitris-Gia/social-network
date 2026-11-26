// Owner: C
// Group posts reuse B's post model and add C's membership gate.

package handlers

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"social-network/pkg/middleware"
	"social-network/pkg/models"
	"social-network/pkg/utils"
)

// CreateGroupPost creates a post in a group. Only confirmed members may post.
func (h *Handler) CreateGroupPost(w http.ResponseWriter, r *http.Request) {
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
		utils.ServerError(w, err)
		return
	}
	if !isMember {
		utils.WriteError(w, http.StatusForbidden, "only group members can post")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, utils.MaxUploadBody)
	if err := r.ParseMultipartForm(utils.MultipartMemory); err != nil {
		postFormError(w, err)
		return
	}
	defer r.MultipartForm.RemoveAll()

	content := strings.TrimSpace(r.FormValue("content"))
	if n := utf8.RuneCountInString(content); n == 0 || n > maxPostLength {
		utils.WriteError(w, http.StatusBadRequest, "content must be 1 to 5000 characters")
		return
	}

	imagePath, status, err := utils.SaveImage(r, "image")
	if err != nil {
		utils.WriteError(w, status, err.Error())
		return
	}

	postID, err := models.CreatePost(h.DB, userID, &groupID, content, imagePath, models.PrivacyPublic, nil)
	if err != nil {
		utils.DeleteImage(imagePath)
		utils.ServerError(w, err)
		return
	}

	post, err := models.GetPost(h.DB, postID)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, post)
}

// ListGroupPosts returns a page of posts to confirmed group members.
func (h *Handler) ListGroupPosts(w http.ResponseWriter, r *http.Request) {
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

	limit, before, valid := postPageParams(w, r)
	if !valid {
		return
	}

	posts, hasMore, err := models.PostsByGroup(h.DB, userID, groupID, limit, before)
	if errors.Is(err, models.ErrPostNotFound) {
		utils.WriteError(w, http.StatusForbidden, "only group members can view posts")
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]any{"posts": posts, "has_more": hasMore})
}
