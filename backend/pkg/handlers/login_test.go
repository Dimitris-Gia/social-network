// Owner: A
package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"social-network/pkg/db/sqlite"
	"social-network/pkg/middleware"
	"social-network/pkg/models"
	"social-network/pkg/websocket"
)

// openAuthTestDB opens a fresh DB with every migration applied. Migrations
// are read relative to backend/, so we step in there just for the Open call.
func openAuthTestDB(t *testing.T) *sql.DB {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	prev, _ := os.Getwd()
	if err := os.Chdir(filepath.Join(filepath.Dir(thisFile), "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prev)

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// do sends one request through the real router (so RequireAuth runs too).
func do(mux http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == middleware.CookieName {
			return c
		}
	}
	return nil
}

func countSessions(t *testing.T, db *sql.DB) int {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLoginLogoutFlow(t *testing.T) {
	db := openAuthTestDB(t)
	db.Exec(`DELETE FROM sessions`) // in case the seed migration added any
	mux := http.NewServeMux()
	RegisterRoutes(mux, db, websocket.NewHub())

	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.MinCost)
	u := &models.User{
		Email: "maria@example.com", PasswordHash: string(hash),
		FirstName: "Maria", LastName: "P", DateOfBirth: "2001-04-17",
	}
	if err := models.CreateUser(db, u); err != nil {
		t.Fatal(err)
	}

	// Wrong password and unknown email: same status, same message.
	w1 := do(mux, "POST", "/api/login", `{"email":"maria@example.com","password":"wrongpass"}`, nil)
	w2 := do(mux, "POST", "/api/login", `{"email":"nobody@example.com","password":"secret123"}`, nil)
	if w1.Code != 401 || w2.Code != 401 || w1.Body.String() != w2.Body.String() {
		t.Fatalf("bad login: %d %s / %d %s", w1.Code, w1.Body, w2.Code, w2.Body)
	}
	if sessionCookie(w1) != nil {
		t.Fatal("failed login must not set a cookie")
	}

	// Missing field and unknown JSON field -> 400.
	if w := do(mux, "POST", "/api/login", `{"email":"maria@example.com"}`, nil); w.Code != 400 {
		t.Fatalf("missing password: got %d", w.Code)
	}
	if w := do(mux, "POST", "/api/login", `{"emial":"x","password":"y"}`, nil); w.Code != 400 {
		t.Fatalf("unknown field: got %d", w.Code)
	}

	// Not logged in -> 401 on a protected route.
	if w := do(mux, "GET", "/api/me", "", nil); w.Code != 401 {
		t.Fatalf("/me without cookie: got %d", w.Code)
	}
	if w := do(mux, "GET", "/api/me", "", &http.Cookie{Name: "session_id", Value: "made-up"}); w.Code != 401 {
		t.Fatalf("/me with fake cookie: got %d", w.Code)
	}

	// Good login (email typed with capitals and spaces).
	w := do(mux, "POST", "/api/login", `{"email":"  Maria@Example.COM ","password":"secret123"}`, nil)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "password") {
		t.Fatal("response leaks the password hash")
	}
	c := sessionCookie(w)
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge != 604800 {
		t.Fatalf("bad cookie: %+v", c)
	}

	// /me with the cookie -> who we are.
	if w := do(mux, "GET", "/api/me", "", c); w.Code != 200 || !strings.Contains(w.Body.String(), `"follow_status":"self"`) {
		t.Fatalf("/me: %d %s", w.Code, w.Body)
	}

	// Logging in again from the same browser replaces the old session.
	w = do(mux, "POST", "/api/login", `{"email":"maria@example.com","password":"secret123"}`, c)
	c2 := sessionCookie(w)
	if c2 == nil || c2.Value == c.Value || countSessions(t, db) != 1 {
		t.Fatalf("re-login should create a new ID and delete the old row (sessions=%d)", countSessions(t, db))
	}
	if w := do(mux, "GET", "/api/me", "", c); w.Code != 401 {
		t.Fatal("old session still works after re-login")
	}

	// Logout: 204, cookie cleared, row gone, cookie useless afterwards.
	w = do(mux, "POST", "/api/logout", "", c2)
	if w.Code != 204 {
		t.Fatalf("logout: %d", w.Code)
	}
	if cl := sessionCookie(w); cl == nil || cl.MaxAge >= 0 {
		t.Fatalf("logout must expire the cookie: %+v", cl)
	}
	if countSessions(t, db) != 0 {
		t.Fatal("logout did not delete the session row")
	}
	if w := do(mux, "GET", "/api/me", "", c2); w.Code != 401 {
		t.Fatal("cookie still works after logout")
	}
	if w := do(mux, "POST", "/api/logout", "", nil); w.Code != 401 {
		t.Fatal("logout without a session should be 401")
	}

	// An expired session is refused even though the row exists.
	db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES ('old', ?, datetime('now', '-1 minute'))`, u.ID)
	if w := do(mux, "GET", "/api/me", "", &http.Cookie{Name: "session_id", Value: "old"}); w.Code != 401 {
		t.Fatal("expired session accepted")
	}
}
