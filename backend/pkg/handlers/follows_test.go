// Owner: A
package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"social-network/pkg/models"
	"social-network/pkg/websocket"
)

// followEnv is a router with a fresh DB and helpers to create logged-in users.
type followEnv struct {
	t   *testing.T
	db  *sql.DB
	mux *http.ServeMux
}

func newFollowEnv(t *testing.T) *followEnv {
	db := openAuthTestDB(t)
	mux := http.NewServeMux()
	RegisterRoutes(mux, db, websocket.NewHub())
	return &followEnv{t: t, db: db, mux: mux}
}

// user registers a user directly in the DB, logs in through the API and
// returns the ID and session cookie.
func (e *followEnv) user(email string, private bool) (int64, *http.Cookie) {
	e.t.Helper()
	db := e.db
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.MinCost)
	u := &models.User{
		Email: email, PasswordHash: string(hash),
		FirstName: email[:1], LastName: "Test", DateOfBirth: "2000-01-01",
	}
	if err := models.CreateUser(db, u); err != nil {
		e.t.Fatal(err)
	}
	if private {
		if err := models.SetPrivacy(db, u.ID, true); err != nil {
			e.t.Fatal(err)
		}
	}
	w := do(e.mux, "POST", "/api/login", fmt.Sprintf(`{"email":%q,"password":"secret123"}`, email), nil)
	c := sessionCookie(w)
	if c == nil {
		e.t.Fatalf("login %s: %d %s", email, w.Code, w.Body)
	}
	return u.ID, c
}

// expect sends a request and fails the test if the status is wrong.
func (e *followEnv) expect(method, path, body string, c *http.Cookie, want int) string {
	e.t.Helper()
	w := do(e.mux, method, path, body, c)
	if w.Code != want {
		e.t.Fatalf("%s %s: want %d, got %d %s", method, path, want, w.Code, w.Body)
	}
	return w.Body.String()
}

func TestFollowPublicAndPrivate(t *testing.T) {
	e := newFollowEnv(t)
	alice, ca := e.user("alice@x.com", false)
	bob, cb := e.user("bob@x.com", true) // private
	_, cc := e.user("carol@x.com", false)

	// Public profile: instant follow.
	body := e.expect("POST", fmt.Sprintf("/api/users/%d/follow", alice), "", cc, 200)
	if !strings.Contains(body, `"following"`) {
		t.Fatalf("public follow should be instant: %s", body)
	}
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", alice), "", cc, 409) // twice
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", alice), "", ca, 400) // yourself
	e.expect("POST", "/api/users/9999/follow", "", ca, 404)

	// Private profile: pending until bob accepts.
	body = e.expect("POST", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 200)
	if !strings.Contains(body, `"pending"`) {
		t.Fatalf("private follow should be pending: %s", body)
	}

	// While pending, alice is locked out of bob's profile and lists.
	body = e.expect("GET", fmt.Sprintf("/api/users/%d", bob), "", ca, 200)
	if !strings.Contains(body, `"can_view":false`) || strings.Contains(body, "bob@x.com") ||
		strings.Contains(body, "date_of_birth") {
		t.Fatalf("locked profile leaks data: %s", body)
	}
	e.expect("GET", fmt.Sprintf("/api/users/%d/followers", bob), "", ca, 403)
	e.expect("GET", fmt.Sprintf("/api/users/%d/following", bob), "", ca, 403)

	// Carol can't accept a request that was sent to bob.
	e.expect("POST", fmt.Sprintf("/api/follow-requests/%d/accept", alice), "", cc, 404)

	// Bob sees the request and accepts it.
	body = e.expect("GET", "/api/follow-requests", "", cb, 200)
	if !strings.Contains(body, `"alice@x.com"`) && !strings.Contains(body, fmt.Sprintf(`"id":%d`, alice)) {
		t.Fatalf("request missing: %s", body)
	}
	e.expect("POST", fmt.Sprintf("/api/follow-requests/%d/accept", alice), "", cb, 204)
	e.expect("POST", fmt.Sprintf("/api/follow-requests/%d/accept", alice), "", cb, 404) // already done

	// Now alice can see everything, and the counts are right.
	body = e.expect("GET", fmt.Sprintf("/api/users/%d", bob), "", ca, 200)
	if !strings.Contains(body, `"can_view":true`) || !strings.Contains(body, `"followers_count":1`) ||
		!strings.Contains(body, `"follow_status":"following"`) {
		t.Fatalf("follower should see the full profile: %s", body)
	}
	e.expect("GET", fmt.Sprintf("/api/users/%d/followers", bob), "", ca, 200)

	// Unfollow, then again -> 404.
	e.expect("DELETE", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 204)
	e.expect("DELETE", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 404)
	e.expect("GET", fmt.Sprintf("/api/users/%d/followers", bob), "", ca, 403)
}

func TestDeclineAndPrivacyToggle(t *testing.T) {
	e := newFollowEnv(t)
	alice, ca := e.user("alice@x.com", false)
	bob, cb := e.user("bob@x.com", true)
	carol, cc := e.user("carol@x.com", false)

	// Decline deletes the request.
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 200)
	e.expect("POST", fmt.Sprintf("/api/follow-requests/%d/decline", alice), "", cb, 204)
	if body := e.expect("GET", "/api/follow-requests", "", cb, 200); body != "[]" {
		t.Fatalf("declined request still listed: %s", body)
	}

	// Going public auto-accepts pending requests.
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", bob), "", cc, 200)
	e.expect("PATCH", "/api/me/privacy", `{"is_private":false}`, cb, 200)
	body := e.expect("GET", fmt.Sprintf("/api/users/%d/followers", bob), "", cb, 200)
	if !strings.Contains(body, fmt.Sprintf(`"id":%d`, carol)) {
		t.Fatalf("pending request should be accepted when going public: %s", body)
	}

	// Bad bodies.
	e.expect("PATCH", "/api/me/privacy", `{}`, cb, 400)
	e.expect("PATCH", "/api/me/privacy", `{"is_private":"yes"}`, cb, 400)
	e.expect("PATCH", "/api/me/privacy", `{"is_private":true}`, nil, 401)
}

func TestListUsersAndMe(t *testing.T) {
	e := newFollowEnv(t)
	alice, ca := e.user("alice@x.com", false)
	bob, _ := e.user("bob@x.com", true)
	e.expect("POST", fmt.Sprintf("/api/users/%d/follow", bob), "", ca, 200)

	body := e.expect("GET", "/api/users", "", ca, 200)
	if strings.Contains(body, fmt.Sprintf(`"id":%d,`, alice)) {
		t.Fatalf("list should not include yourself: %s", body)
	}
	if !strings.Contains(body, `"follow_status":"pending"`) || strings.Contains(body, "email") {
		t.Fatalf("bad user list: %s", body)
	}

	body = e.expect("GET", "/api/me", "", ca, 200)
	if !strings.Contains(body, `"following_count":0`) { // pending doesn't count
		t.Fatalf("/me counts: %s", body)
	}
	e.expect("GET", "/api/users/abc", "", ca, 400)
	e.expect("GET", "/api/users/9999", "", ca, 404)
}
