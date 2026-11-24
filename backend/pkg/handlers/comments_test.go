// Owner: B
package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Uses the helpers from posts_test.go (newPostsEnv, postForm, decode, ...).

// newPost creates a post through the API and returns its comments URL.
func (e *postsEnv) newPost(t *testing.T, cookie *http.Cookie, privacy string) (postID int, commentsPath string) {
	t.Helper()
	rec := e.postForm("/api/posts", map[string]string{"content": "a post", "privacy": privacy}, nil, cookie)
	wantStatus(t, rec, http.StatusCreated)
	postID = int(decode(t, rec)["id"].(float64))
	return postID, "/api/posts/" + strconv.Itoa(postID) + "/comments"
}

func TestCommentsRequireLogin(t *testing.T) {
	e := newPostsEnv(t)
	wantStatus(t, e.get("/api/posts/1/comments", nil), http.StatusUnauthorized)
	wantStatus(t, e.postForm("/api/posts/1/comments", map[string]string{"content": "x"}, nil, nil), http.StatusUnauthorized)
}

func TestCreateAndListCommentsHandler(t *testing.T) {
	e := newPostsEnv(t)
	me, cookie := e.user("me")
	postID, path := e.newPost(t, cookie, "public")

	// an empty post has "comments": [] (not null)
	rec := e.get(path, cookie)
	wantStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"comments":[]`) {
		t.Errorf("body = %s", rec.Body)
	}

	rec = e.postForm(path, map[string]string{"content": "  first  "}, tinyGIF, cookie)
	wantStatus(t, rec, http.StatusCreated)
	c := decode(t, rec)
	if c["content"] != "first" || int(c["post_id"].(float64)) != postID {
		t.Errorf("unexpected comment: %v", c)
	}
	img, _ := c["image_path"].(string)
	if !strings.HasPrefix(img, "/uploads/") || !strings.HasSuffix(img, ".gif") {
		t.Errorf("image_path = %q, want /uploads/<uuid>.gif", img)
	}
	if author := c["author"].(map[string]any); int64(author["id"].(float64)) != me {
		t.Errorf("author = %v, want user %d", author, me)
	}

	wantStatus(t, e.postForm(path, map[string]string{"content": "second"}, nil, cookie), http.StatusCreated)

	list := decode(t, e.get(path, cookie))["comments"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["content"] != "first" || list[1].(map[string]any)["content"] != "second" {
		t.Errorf("comments = %v, want [first, second] oldest first", list)
	}

	// the counter on the post follows
	post := decode(t, e.get("/api/posts/"+strconv.Itoa(postID), cookie))
	if post["comment_count"].(float64) != 2 {
		t.Errorf("comment_count = %v, want 2", post["comment_count"])
	}
}

func TestCreateCommentValidation(t *testing.T) {
	e := newPostsEnv(t)
	_, cookie := e.user("me")
	_, path := e.newPost(t, cookie, "public")

	bad := []struct {
		name   string
		fields map[string]string
		file   []byte
		status int
	}{
		{"empty content", map[string]string{"content": "   "}, nil, 400},
		{"too long", map[string]string{"content": strings.Repeat("a", 2001)}, nil, 400},
		{"not an image", map[string]string{"content": "x"}, notAnImage, 415},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			before := filesInUploadDir(t)
			wantStatus(t, e.postForm(path, tc.fields, tc.file, cookie), tc.status)
			if filesInUploadDir(t) != before {
				t.Errorf("a rejected comment left a file in the upload folder")
			}
		})
	}

	wantStatus(t, e.postForm("/api/posts/abc/comments", map[string]string{"content": "x"}, nil, cookie), http.StatusBadRequest)
	wantStatus(t, e.get("/api/posts/abc/comments", cookie), http.StatusBadRequest)
}

func TestCommentsOnHiddenPostHandler(t *testing.T) {
	e := newPostsEnv(t)
	author, authorCookie := e.user("author")
	_, strangerCookie := e.user("stranger")
	follower, followerCookie := e.user("follower")
	e.follow(follower, author)
	_, path := e.newPost(t, authorCookie, "almost_private")

	// a stranger can neither read nor write, and a rejected image is removed again
	before := filesInUploadDir(t)
	hiddenPost := e.postForm(path, map[string]string{"content": "hi"}, tinyGIF, strangerCookie)
	wantStatus(t, hiddenPost, http.StatusNotFound)
	if filesInUploadDir(t) != before {
		t.Errorf("image of a rejected comment was left on disk")
	}
	hiddenGet := e.get(path, strangerCookie)
	wantStatus(t, hiddenGet, http.StatusNotFound)

	// ... and the answer is the same as for a post that does not exist
	missingGet := e.get("/api/posts/9999/comments", strangerCookie)
	wantStatus(t, missingGet, http.StatusNotFound)
	if hiddenGet.Body.String() != missingGet.Body.String() {
		t.Errorf("hidden and missing posts must look identical:\n%s\n%s", hiddenGet.Body, missingGet.Body)
	}

	var n int
	e.db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&n)
	if n != 0 {
		t.Errorf("%d comments stored by a stranger, want 0", n)
	}

	// the follower can do both
	wantStatus(t, e.postForm(path, map[string]string{"content": "hi"}, nil, followerCookie), http.StatusCreated)
	wantStatus(t, e.get(path, followerCookie), http.StatusOK)
}
