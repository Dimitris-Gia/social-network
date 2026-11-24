// Owner: B
package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"social-network/pkg/db/sqlite"
	"social-network/pkg/middleware"
	"social-network/pkg/utils"
)

// These tests go through the real routes and the real RequireAuth: a session
// row is inserted in the database and its ID is sent as the cookie.

type postsEnv struct {
	t   *testing.T
	db  *sql.DB
	mux *http.ServeMux
}

func newPostsEnv(t *testing.T) *postsEnv {
	t.Helper()
	// sqlite.Open reads migrations relative to backend/: step in, open, step out.
	_, thisFile, _, _ := runtime.Caller(0)
	backendDir := filepath.Join(filepath.Dir(thisFile), "..", "..")
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(backendDir); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	os.Chdir(prev)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// images go to a temp folder, not into backend/uploads
	oldDir := utils.UploadDir
	utils.UploadDir = filepath.Join(t.TempDir(), "uploads")
	t.Cleanup(func() { utils.UploadDir = oldDir })

	mux := http.NewServeMux()
	RegisterRoutes(mux, db)
	return &postsEnv{t: t, db: db, mux: mux}
}

// user inserts a user and a valid session; it returns the user ID and cookie.
func (e *postsEnv) user(name string) (int64, *http.Cookie) {
	e.t.Helper()
	res, err := e.db.Exec(`INSERT INTO users (email, password_hash, first_name, last_name, date_of_birth)
		VALUES (?, 'x', ?, 'Test', '2000-01-01')`, name+"@test.com", name)
	if err != nil {
		e.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	sid := "session-" + name
	if _, err := e.db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, datetime('now', '+1 day'))`, sid, id); err != nil {
		e.t.Fatal(err)
	}
	return id, &http.Cookie{Name: middleware.CookieName, Value: sid}
}

func (e *postsEnv) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
}

func (e *postsEnv) follow(follower, following int64) {
	e.exec(`INSERT INTO follows (follower_id, following_id, status) VALUES (?, ?, 'accepted')`, follower, following)
}

func (e *postsEnv) do(req *http.Request, cookie *http.Cookie) *httptest.ResponseRecorder {
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

func (e *postsEnv) get(path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	return e.do(httptest.NewRequest(http.MethodGet, path, nil), cookie)
}

// postForm sends POST path as multipart/form-data with optional file.
func (e *postsEnv) postForm(path string, fields map[string]string, file []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	if file != nil {
		fw, _ := mw.CreateFormFile("image", "whatever.bin")
		fw.Write(file)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return e.do(req, cookie)
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON: %q", rec.Body.String())
	}
	return m
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, want, rec.Body.String())
	}
}

func filesInUploadDir(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(utils.UploadDir)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

var (
	tinyGIF    = []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;")
	notAnImage = []byte("this is plain text, not an image")
)

func TestPostsRequireLogin(t *testing.T) {
	e := newPostsEnv(t)
	for _, path := range []string{"/api/posts/feed", "/api/posts/1", "/api/users/1/posts"} {
		wantStatus(t, e.get(path, nil), http.StatusUnauthorized)
	}
	wantStatus(t, e.postForm("/api/posts", map[string]string{"content": "x"}, nil, nil), http.StatusUnauthorized)
}

func TestCreatePost(t *testing.T) {
	e := newPostsEnv(t)
	me, cookie := e.user("me")

	t.Run("public post with image", func(t *testing.T) {
		rec := e.postForm("/api/posts", map[string]string{"content": "  hello  "}, tinyGIF, cookie)
		wantStatus(t, rec, http.StatusCreated)
		post := decode(t, rec)
		if post["content"] != "hello" || post["privacy"] != "public" {
			t.Errorf("unexpected post: %v", post)
		}
		img, _ := post["image_path"].(string)
		if !strings.HasPrefix(img, "/uploads/") || !strings.HasSuffix(img, ".gif") {
			t.Errorf("image_path = %q, want /uploads/<uuid>.gif", img)
		}
		if author := post["author"].(map[string]any); int64(author["id"].(float64)) != me {
			t.Errorf("author = %v, want user %d", author, me)
		}
		if filesInUploadDir(t) != 1 {
			t.Errorf("want exactly 1 saved image")
		}
	})

	t.Run("user id in the form is ignored", func(t *testing.T) {
		other, _ := e.user("other")
		rec := e.postForm("/api/posts", map[string]string{"content": "x", "user_id": strconv.Itoa(int(other))}, nil, cookie)
		wantStatus(t, rec, http.StatusCreated)
		author := decode(t, rec)["author"].(map[string]any)
		if int64(author["id"].(float64)) != me {
			t.Errorf("author = %v, must be the logged-in user %d", author, me)
		}
	})

	bad := []struct {
		name   string
		fields map[string]string
		file   []byte
		status int
	}{
		{"empty content", map[string]string{"content": "   "}, nil, 400},
		{"too long", map[string]string{"content": strings.Repeat("a", 5001)}, nil, 400},
		{"unknown privacy", map[string]string{"content": "x", "privacy": "secret"}, nil, 400},
		{"private without viewers", map[string]string{"content": "x", "privacy": "private"}, nil, 400},
		{"viewer list is not numbers", map[string]string{"content": "x", "privacy": "private", "allowed_viewers": "a,b"}, nil, 400},
		{"viewer who does not follow me", map[string]string{"content": "x", "privacy": "private", "allowed_viewers": "2"}, nil, 400},
		{"not an image", map[string]string{"content": "x"}, notAnImage, 415},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			before := filesInUploadDir(t)
			rec := e.postForm("/api/posts", tc.fields, tc.file, cookie)
			wantStatus(t, rec, tc.status)
			if decode(t, rec)["error"] == "" {
				t.Errorf("error body missing: %s", rec.Body.String())
			}
			if filesInUploadDir(t) != before {
				t.Errorf("a rejected post left a file in the upload folder")
			}
		})
	}

	t.Run("failure after the image was saved removes the image", func(t *testing.T) {
		// the viewer check only happens inside the model, after SaveImage
		stranger, _ := e.user("stranger")
		before := filesInUploadDir(t)
		rec := e.postForm("/api/posts", map[string]string{
			"content": "x", "privacy": "private", "allowed_viewers": strconv.Itoa(int(stranger)),
		}, tinyGIF, cookie)
		wantStatus(t, rec, http.StatusBadRequest)
		if filesInUploadDir(t) != before {
			t.Errorf("orphan image left behind")
		}
	})

	t.Run("JSON body is refused", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/posts", strings.NewReader(`{"content":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		wantStatus(t, e.do(req, cookie), http.StatusBadRequest)
	})

	t.Run("private post for a follower", func(t *testing.T) {
		fan, _ := e.user("fan")
		e.follow(fan, me)
		rec := e.postForm("/api/posts", map[string]string{
			"content": "secret", "privacy": "private", "allowed_viewers": strconv.Itoa(int(fan)),
		}, nil, cookie)
		wantStatus(t, rec, http.StatusCreated)
	})
}

func TestGetPostVisibility(t *testing.T) {
	e := newPostsEnv(t)
	author, authorCookie := e.user("author")
	_, strangerCookie := e.user("stranger")
	follower, followerCookie := e.user("follower")
	e.follow(follower, author)

	rec := e.postForm("/api/posts", map[string]string{"content": "friends only", "privacy": "almost_private"}, nil, authorCookie)
	wantStatus(t, rec, http.StatusCreated)
	path := "/api/posts/" + strconv.Itoa(int(decode(t, rec)["id"].(float64)))

	wantStatus(t, e.get(path, authorCookie), http.StatusOK)
	wantStatus(t, e.get(path, followerCookie), http.StatusOK)
	// a stranger gets the same answer as for a post that does not exist
	hidden := e.get(path, strangerCookie)
	wantStatus(t, hidden, http.StatusNotFound)
	missing := e.get("/api/posts/9999", strangerCookie)
	wantStatus(t, missing, http.StatusNotFound)
	if hidden.Body.String() != missing.Body.String() {
		t.Errorf("hidden and missing posts must look identical:\n%s\n%s", hidden.Body, missing.Body)
	}

	wantStatus(t, e.get("/api/posts/abc", authorCookie), http.StatusBadRequest)
}

func TestFeedHandler(t *testing.T) {
	e := newPostsEnv(t)
	_, cookie := e.user("me")
	for i := 0; i < 3; i++ {
		wantStatus(t, e.postForm("/api/posts", map[string]string{"content": "p" + strconv.Itoa(i)}, nil, cookie), http.StatusCreated)
	}

	rec := e.get("/api/posts/feed?limit=2", cookie)
	wantStatus(t, rec, http.StatusOK)
	body := decode(t, rec)
	posts := body["posts"].([]any)
	if len(posts) != 2 || body["has_more"] != true {
		t.Fatalf("page 1: %d posts, has_more=%v", len(posts), body["has_more"])
	}
	if posts[0].(map[string]any)["content"] != "p2" {
		t.Errorf("newest post should come first: %v", posts[0])
	}

	lastID := int(posts[1].(map[string]any)["id"].(float64))
	body = decode(t, e.get("/api/posts/feed?limit=2&before="+strconv.Itoa(lastID), cookie))
	if len(body["posts"].([]any)) != 1 || body["has_more"] != false {
		t.Errorf("page 2 = %v", body)
	}

	for _, q := range []string{"limit=0", "limit=abc", "before=-1", "before=x"} {
		wantStatus(t, e.get("/api/posts/feed?"+q, cookie), http.StatusBadRequest)
	}

	// a new user with nothing to see gets [] and not null
	_, emptyCookie := e.user("empty")
	rec = e.get("/api/posts/feed", emptyCookie)
	if !strings.Contains(rec.Body.String(), `"posts":[`) {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestUserPostsHandler(t *testing.T) {
	e := newPostsEnv(t)
	author, authorCookie := e.user("author")
	_, strangerCookie := e.user("stranger")
	follower, followerCookie := e.user("follower")
	e.follow(follower, author)
	wantStatus(t, e.postForm("/api/posts", map[string]string{"content": "public one"}, nil, authorCookie), http.StatusCreated)
	wantStatus(t, e.postForm("/api/posts", map[string]string{"content": "friends", "privacy": "almost_private"}, nil, authorCookie), http.StatusCreated)

	path := "/api/users/" + strconv.Itoa(int(author)) + "/posts"

	count := func(c *http.Cookie) int {
		rec := e.get(path, c)
		wantStatus(t, rec, http.StatusOK)
		return len(decode(t, rec)["posts"].([]any))
	}
	if n := count(strangerCookie); n != 1 {
		t.Errorf("stranger sees %d posts, want 1", n)
	}
	if n := count(followerCookie); n != 2 {
		t.Errorf("follower sees %d posts, want 2", n)
	}

	// private profile: the profile gate answers 403 before any post is looked at
	e.exec(`UPDATE users SET is_private = 1 WHERE id = ?`, author)
	wantStatus(t, e.get(path, strangerCookie), http.StatusForbidden)
	wantStatus(t, e.get(path, followerCookie), http.StatusOK)
	wantStatus(t, e.get(path, authorCookie), http.StatusOK)

	wantStatus(t, e.get("/api/users/9999/posts", strangerCookie), http.StatusNotFound)
	wantStatus(t, e.get("/api/users/abc/posts", strangerCookie), http.StatusBadRequest)
}
