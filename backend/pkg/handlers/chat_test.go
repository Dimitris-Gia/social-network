// Owner: B
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"

	"social-network/pkg/middleware"
	"social-network/pkg/websocket"
)

// Uses newPostsEnv from posts_test.go: real routes, real RequireAuth.

// dialWS opens GET /ws against a real HTTP server. It returns the connection
// (nil if the server refused the upgrade) and the HTTP response.
func dialWS(t *testing.T, e *postsEnv, query string, header http.Header) (*gws.Conn, *http.Response) {
	t.Helper()
	srv := httptest.NewServer(e.mux)
	t.Cleanup(srv.Close)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws" + query
	conn, resp, err := gws.DefaultDialer.Dial(url, header)
	if err != nil {
		if resp == nil {
			t.Fatalf("dial failed without a response: %v", err)
		}
		return nil, resp
	}
	t.Cleanup(func() { conn.Close() })
	return conn, resp
}

func cookieHeader(c *http.Cookie) http.Header {
	h := http.Header{}
	h.Set("Cookie", c.Name+"="+c.Value)
	return h
}

func waitOnline(t *testing.T, e *postsEnv, userID int64, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if e.hub.IsOnline(userID) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("user %d: IsOnline never became %v", userID, want)
}

func readWSFrame(t *testing.T, conn *gws.Conn) (typ string, data json.RawMessage) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var f struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := conn.ReadJSON(&f); err != nil {
		t.Fatalf("expected a frame: %v", err)
	}
	return f.Type, f.Data
}

func TestWebSocketNeedsLogin(t *testing.T) {
	e := newPostsEnv(t)

	conn, resp := dialWS(t, e, "", nil)
	if conn != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no cookie: conn=%v status=%d, want a 401 and no connection", conn != nil, resp.StatusCode)
	}

	bad := cookieHeader(&http.Cookie{Name: middleware.CookieName, Value: "not-a-session"})
	conn, resp = dialWS(t, e, "", bad)
	if conn != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad cookie: conn=%v status=%d, want a 401 and no connection", conn != nil, resp.StatusCode)
	}
}

func TestWebSocketUsesTheSessionUser(t *testing.T) {
	e := newPostsEnv(t)
	me, cookie := e.user("me")

	// the URL claims to be user 999: only the session counts
	conn, _ := dialWS(t, e, "?user=999&user_id=999", cookieHeader(cookie))
	if conn == nil {
		t.Fatal("logged-in user could not connect")
	}
	waitOnline(t, e, me, true)
	if e.hub.IsOnline(999) {
		t.Error("the hub trusted a user ID from the URL")
	}

	// server -> browser
	e.hub.SendToUser(me, websocket.Message{Type: "notification", Data: map[string]int{"id": 1}})
	if typ, data := readWSFrame(t, conn); typ != "notification" || string(data) != `{"id":1}` {
		t.Errorf("got %q %s", typ, data)
	}

	// browser -> server: no handler registered yet, so an error frame comes back
	conn.WriteJSON(map[string]any{"type": "private_message", "data": map[string]any{}})
	if typ, _ := readWSFrame(t, conn); typ != "error" {
		t.Errorf("got %q, want an error frame", typ)
	}

	// leaving unregisters the user
	conn.Close()
	waitOnline(t, e, me, false)
}

// A page on another website must not be able to open a socket with the
// user's cookie (cross-site WebSocket hijacking).
func TestWebSocketRefusesOtherOrigins(t *testing.T) {
	e := newPostsEnv(t)
	_, cookie := e.user("me")

	header := cookieHeader(cookie)
	header.Set("Origin", "http://evil.example")
	conn, resp := dialWS(t, e, "", header)
	if conn != nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign origin: conn=%v status=%d, want a 403 and no connection", conn != nil, resp.StatusCode)
	}
}

func TestWebSocketTwoUsersStayApart(t *testing.T) {
	e := newPostsEnv(t)
	a, cookieA := e.user("a")
	b, cookieB := e.user("b")
	connA, _ := dialWS(t, e, "", cookieHeader(cookieA))
	connB, _ := dialWS(t, e, "", cookieHeader(cookieB))
	waitOnline(t, e, a, true)
	waitOnline(t, e, b, true)

	e.hub.SendToUser(b, websocket.Message{Type: "for_b"})
	if typ, _ := readWSFrame(t, connB); typ != "for_b" {
		t.Errorf("b got %q", typ)
	}
	connA.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := connA.ReadMessage(); err == nil {
		t.Error("user a received a frame meant for user b " + strconv.Itoa(int(b)))
	}
}
