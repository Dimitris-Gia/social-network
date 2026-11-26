// Owner: B
// create post, feed, single post, user's posts
//
// Handlers only decode the request, check permissions, call pkg/models and
// answer with JSON. No SQL here. Helper names start with "post" so they
// cannot clash with helpers other owners add to this package.

package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"social-network/pkg/middleware"
	"social-network/pkg/models"
	"social-network/pkg/utils"
)

const maxPostLength = 5000 // characters (API.md)

// CreatePost handles POST /api/posts (multipart/form-data).
func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)

	r.Body = http.MaxBytesReader(w, r.Body, utils.MaxUploadBody)
	if err := r.ParseMultipartForm(utils.MultipartMemory); err != nil {
		postFormError(w, err)
		return
	}
	defer r.MultipartForm.RemoveAll() // delete Go's temp files for big uploads

	// Cheap checks first, so we do not write an image to disk for a bad request.
	content := strings.TrimSpace(r.FormValue("content"))
	if n := utf8.RuneCountInString(content); n == 0 || n > maxPostLength {
		utils.WriteError(w, http.StatusBadRequest, "content must be 1 to 5000 characters")
		return
	}

	privacy := r.FormValue("privacy")
	if privacy == "" {
		privacy = models.PrivacyPublic
	}
	switch privacy {
	case models.PrivacyPublic, models.PrivacyAlmostPrivate, models.PrivacyPrivate:
	default:
		utils.WriteError(w, http.StatusBadRequest, "privacy must be public, almost_private or private")
		return
	}

	viewers, err := postParseIDList(r.FormValue("allowed_viewers"))
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "allowed_viewers must be comma-separated user IDs")
		return
	}

	imagePath, status, err := utils.SaveImage(r, "image")
	if err != nil {
		utils.WriteError(w, status, err.Error())
		return
	}

	id, err := models.CreatePost(h.DB, userID, nil, content, imagePath, privacy, viewers)
	if err != nil {
		utils.DeleteImage(imagePath) // do not leave an orphan file behind
		switch {
		case errors.Is(err, models.ErrNoViewers):
			utils.WriteError(w, http.StatusBadRequest, "choose at least one viewer for a private post")
		case errors.Is(err, models.ErrViewerNotFollower):
			utils.WriteError(w, http.StatusBadRequest, "every chosen viewer must be one of your followers")
		case errors.Is(err, models.ErrInvalidPrivacy):
			utils.WriteError(w, http.StatusBadRequest, "privacy must be public, almost_private or private")
		default:
			utils.ServerError(w, err)
		}
		return
	}

	post, err := models.GetPost(h.DB, id)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, post)
}

// Feed handles GET /api/posts/feed?limit=&before=.
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	limit, before, ok := postPageParams(w, r)
	if !ok {
		return
	}

	posts, hasMore, err := models.Feed(h.DB, userID, limit, before)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]any{"posts": posts, "has_more": hasMore})
}

// GetPost handles GET /api/posts/{id}. A post that does not exist and a post
// you may not see both answer 404, so hidden posts cannot be discovered.
func (h *Handler) GetPost(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	id, ok := postPathID(w, r, "post")
	if !ok {
		return
	}

	post, err := models.GetPost(h.DB, id)
	if errors.Is(err, models.ErrPostNotFound) {
		utils.WriteError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}

	canView, err := models.CanViewPost(h.DB, userID, post)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if !canView {
		utils.WriteError(w, http.StatusNotFound, "post not found")
		return
	}
	utils.WriteJSON(w, http.StatusOK, post)
}

// UserPosts handles GET /api/users/{id}/posts?limit=&before=.
// Two gates: the PROFILE rule first (403 on a private profile), then the
// per-post rule inside models.PostsByUser.
func (h *Handler) UserPosts(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	authorID, ok := postPathID(w, r, "user")
	if !ok {
		return
	}
	limit, before, ok := postPageParams(w, r)
	if !ok {
		return
	}

	canView, err := models.CanViewProfile(h.DB, userID, authorID)
	if errors.Is(err, models.ErrUserNotFound) {
		utils.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if !canView {
		utils.WriteError(w, http.StatusForbidden, "this profile is private")
		return
	}

	posts, hasMore, err := models.PostsByUser(h.DB, userID, authorID, limit, before)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]any{"posts": posts, "has_more": hasMore})
}

// postFormError answers for a request whose body could not be parsed.
func postFormError(w http.ResponseWriter, err error) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		utils.WriteError(w, http.StatusRequestEntityTooLarge, "request is too large")
		return
	}
	utils.WriteError(w, http.StatusBadRequest, "send the post as multipart/form-data")
}

// postPathID reads the {id} path value. what is only used in the message.
func postPathID(w http.ResponseWriter, r *http.Request, what string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		utils.WriteError(w, http.StatusBadRequest, "invalid "+what+" id")
		return 0, false
	}
	return id, true
}

// postPageParams reads ?limit= and ?before=. Missing means 0, which the model
// turns into "default page size" and "start from the newest".
func postPageParams(w http.ResponseWriter, r *http.Request) (limit int, before int64, ok bool) {
	q := r.URL.Query()
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			utils.WriteError(w, http.StatusBadRequest, "limit must be a positive number")
			return 0, 0, false
		}
		limit = n
	}
	if s := q.Get("before"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n < 1 {
			utils.WriteError(w, http.StatusBadRequest, "before must be a post ID")
			return 0, 0, false
		}
		before = n
	}
	return limit, before, true
}

// postParseIDList turns "4,9,12" into []int64{4, 9, 12}. "" gives no IDs.
func postParseIDList(s string) ([]int64, error) {
	var ids []int64
	if strings.TrimSpace(s) == "" {
		return ids, nil
	}
	for _, part := range strings.Split(s, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id < 1 {
			return nil, errors.New("bad id")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
