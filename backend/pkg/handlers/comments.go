// Owner: B
// create + list comments
//
// The visibility rule lives in models.CreateComment and models.CommentsByPost
// (they run CanViewPost themselves): a post you cannot see answers
// ErrPostNotFound, which becomes the same 404 as a post that does not exist.

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

const maxCommentLength = 2000 // characters (API.md)

// CreateComment handles POST /api/posts/{id}/comments (multipart/form-data).
func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	postID, ok := postPathID(w, r, "post")
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, utils.MaxUploadBody)
	if err := r.ParseMultipartForm(utils.MultipartMemory); err != nil {
		postFormError(w, err)
		return
	}
	defer r.MultipartForm.RemoveAll() // delete Go's temp files for big uploads

	// Cheap check first, so we do not write an image to disk for a bad request.
	content := strings.TrimSpace(r.FormValue("content"))
	if n := utf8.RuneCountInString(content); n == 0 || n > maxCommentLength {
		utils.WriteError(w, http.StatusBadRequest, "content must be 1 to 2000 characters")
		return
	}

	imagePath, status, err := utils.SaveImage(r, "image")
	if err != nil {
		utils.WriteError(w, status, err.Error())
		return
	}

	comment, err := models.CreateComment(h.DB, postID, userID, content, imagePath)
	if err != nil {
		utils.DeleteImage(imagePath) // do not leave an orphan file behind
		if errors.Is(err, models.ErrPostNotFound) {
			utils.WriteError(w, http.StatusNotFound, "post not found")
			return
		}
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, comment)
}

// ListComments handles GET /api/posts/{id}/comments (oldest first).
func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	postID, ok := postPathID(w, r, "post")
	if !ok {
		return
	}

	comments, err := models.CommentsByPost(h.DB, userID, postID)
	if errors.Is(err, models.ErrPostNotFound) {
		utils.WriteError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]any{"comments": comments})
}
