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
	"social-network/pkg/models"
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

// ---- private chat ----

func wsSend(t *testing.T, conn *gws.Conn, to int64, content string, extra map[string]any) {
	t.Helper()
	data := map[string]any{"to": to, "content": content}
	for k, v := range extra {
		data[k] = v
	}
	if err := conn.WriteJSON(map[string]any{"type": "private_message", "data": data}); err != nil {
		t.Fatal(err)
	}
}

func expectNoFrame(t *testing.T, conn *gws.Conn, who string) { // last read on that connection
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, raw, err := conn.ReadMessage(); err == nil {
		t.Errorf("%s received a frame it should not get: %s", who, raw)
	}
}

func countMessages(t *testing.T, e *postsEnv) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *postsEnv) json(t *testing.T, path string, cookie *http.Cookie, want int) map[string]any {
	t.Helper()
	rec := e.get(path, cookie)
	wantStatus(t, rec, want)
	return decode(t, rec)
}

func TestChatOverWebSocket(t *testing.T) {
	e := newPostsEnv(t)
	a, cookieA := e.user("a")
	b, cookieB := e.user("b")
	c, cookieC := e.user("c") // follows nobody, nobody follows them
	e.follow(a, b)            // a follows b: enough for both to chat

	connA, _ := dialWS(t, e, "", cookieHeader(cookieA))
	connB, _ := dialWS(t, e, "", cookieHeader(cookieB))
	connC, _ := dialWS(t, e, "", cookieHeader(cookieC))
	waitOnline(t, e, a, true)
	waitOnline(t, e, b, true)
	waitOnline(t, e, c, true)

	// the frame lies about who sent it: the connection's user counts
	wsSend(t, connA, b, "  Hi 😀  ", map[string]any{"sender_id": c})

	typ, data := readWSFrame(t, connB)
	var got models.ChatMessage
	json.Unmarshal(data, &got)
	if typ != "private_message" || got.Content != "Hi 😀" || got.SenderID != a || got.ReceiverID != b || got.IsRead {
		t.Errorf("recipient got %q %+v", typ, got)
	}
	// the sender's own tab gets the same message (so a second tab stays in sync)
	typ, data = readWSFrame(t, connA)
	var echo models.ChatMessage
	json.Unmarshal(data, &echo)
	if typ != "private_message" || echo.ID != got.ID {
		t.Errorf("sender got %q %+v, want the same message", typ, echo)
	}
	if countMessages(t, e) != 1 {
		t.Errorf("want exactly 1 stored message")
	}

	// the REST side sees it: badge, history, contacts
	if n := e.json(t, "/api/chat/unread-count", cookieB, 200)["unread_count"]; n != 1.0 {
		t.Errorf("b unread = %v, want 1", n)
	}
	hist := e.json(t, "/api/chat/"+strconv.Itoa(int(a))+"/messages", cookieB, 200)["messages"].([]any)
	if len(hist) != 1 || hist[0].(map[string]any)["content"] != "Hi 😀" {
		t.Errorf("history = %v", hist)
	}
	contacts := e.json(t, "/api/chat/contacts", cookieB, 200)["contacts"].([]any)
	if len(contacts) != 1 || contacts[0].(map[string]any)["unread_count"] != 1.0 {
		t.Errorf("contacts = %v", contacts)
	}
	wantStatus(t, e.do(httptest.NewRequest(http.MethodPost, "/api/chat/"+strconv.Itoa(int(a))+"/read", nil), cookieB), http.StatusNoContent)
	if n := e.json(t, "/api/chat/unread-count", cookieB, 200)["unread_count"]; n != 0.0 {
		t.Errorf("b unread after reading = %v, want 0", n)
	}

	// only the targeted user got anything
	expectNoFrame(t, connC, "user c")
}

func TestChatRefusedWhenNotFollowing(t *testing.T) {
	e := newPostsEnv(t)
	a, cookieA := e.user("a")
	b, cookieB := e.user("b")
	c, cookieC := e.user("c") // unrelated to a
	e.follow(a, b)

	connA, _ := dialWS(t, e, "", cookieHeader(cookieA))
	connB, _ := dialWS(t, e, "", cookieHeader(cookieB))
	connC, _ := dialWS(t, e, "", cookieHeader(cookieC))
	waitOnline(t, e, a, true)
	waitOnline(t, e, b, true)
	waitOnline(t, e, c, true)

	wsSend(t, connA, c, "hello stranger", nil)
	if typ, _ := readWSFrame(t, connA); typ != "error" {
		t.Errorf("got %q, want an error frame", typ)
	}
	if countMessages(t, e) != 0 {
		t.Errorf("a refused message was stored")
	}

	// the connection is still alive and a permitted message still works
	wsSend(t, connA, b, "hello friend", nil)
	if typ, _ := readWSFrame(t, connB); typ != "private_message" {
		t.Errorf("b got %q, want the message", typ)
	}
	if typ, _ := readWSFrame(t, connA); typ != "private_message" {
		t.Errorf("a got %q, want its own echo", typ)
	}
	expectNoFrame(t, connC, "user c")
}

func TestChatBadFramesOverWebSocket(t *testing.T) {
	e := newPostsEnv(t)
	a, cookieA := e.user("a")
	b, _ := e.user("b")
	e.follow(a, b)
	connA, _ := dialWS(t, e, "", cookieHeader(cookieA))
	waitOnline(t, e, a, true)

	raw := func(frame string) {
		if err := connA.WriteMessage(gws.TextMessage, []byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	for name, frame := range map[string]string{
		"no data":            `{"type":"private_message"}`,
		"no recipient":       `{"type":"private_message","data":{"content":"x"}}`,
		"empty content":      `{"type":"private_message","data":{"to":` + strconv.Itoa(int(b)) + `,"content":"   "}}`,
		"too long":           `{"type":"private_message","data":{"to":` + strconv.Itoa(int(b)) + `,"content":"` + strings.Repeat("a", 2001) + `"}}`,
		"to myself":          `{"type":"private_message","data":{"to":` + strconv.Itoa(int(a)) + `,"content":"x"}}`,
		"unknown user":       `{"type":"private_message","data":{"to":9999,"content":"x"}}`,
		"to is not a number": `{"type":"private_message","data":{"to":"b","content":"x"}}`,
	} {
		raw(frame)
		if typ, _ := readWSFrame(t, connA); typ != "error" {
			t.Errorf("%s: got %q, want an error frame", name, typ)
		}
	}
	if countMessages(t, e) != 0 {
		t.Errorf("a bad frame was stored")
	}

	// 2000 characters of emoji are 2000 characters: the limit counts characters, not bytes
	wsSend(t, connA, b, strings.Repeat("😀", 2000), nil)
	if typ, _ := readWSFrame(t, connA); typ != "private_message" {
		t.Errorf("2000 emoji: got %q, want it accepted", typ)
	}
}

func TestChatRESTEndpoints(t *testing.T) {
	e := newPostsEnv(t)
	a, cookieA := e.user("a")
	b, _ := e.user("b")
	stranger, _ := e.user("stranger")
	e.follow(a, b)
	idB, idStranger := strconv.Itoa(int(b)), strconv.Itoa(int(stranger))

	// login required everywhere
	for _, path := range []string{"/api/chat/contacts", "/api/chat/unread-count", "/api/chat/" + idB + "/messages"} {
		wantStatus(t, e.get(path, nil), http.StatusUnauthorized)
	}
	wantStatus(t, e.do(httptest.NewRequest(http.MethodPost, "/api/chat/"+idB+"/read", nil), nil), http.StatusUnauthorized)

	// related users: an empty conversation is [] and has_more false
	rec := e.get("/api/chat/"+idB+"/messages", cookieA)
	wantStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"messages":[]`) {
		t.Errorf("body = %s", rec.Body)
	}

	for i := 0; i < 3; i++ {
		if _, err := models.SavePrivateMessage(e.db, a, b, "m"+strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	page := e.json(t, "/api/chat/"+idB+"/messages?limit=2", cookieA, 200)
	msgs := page["messages"].([]any)
	if len(msgs) != 2 || page["has_more"] != true || msgs[1].(map[string]any)["content"] != "m2" {
		t.Errorf("page = %v", page)
	}

	// strangers are refused on every chat route, and nothing is changed
	wantStatus(t, e.get("/api/chat/"+idStranger+"/messages", cookieA), http.StatusForbidden)
	wantStatus(t, e.do(httptest.NewRequest(http.MethodPost, "/api/chat/"+idStranger+"/read", nil), cookieA), http.StatusForbidden)
	contacts := e.json(t, "/api/chat/contacts", cookieA, 200)["contacts"].([]any)
	if len(contacts) != 1 {
		t.Errorf("contacts = %v, want only b", contacts)
	}

	wantStatus(t, e.get("/api/chat/9999/messages", cookieA), http.StatusNotFound)
	wantStatus(t, e.get("/api/chat/abc/messages", cookieA), http.StatusBadRequest)
	wantStatus(t, e.get("/api/chat/"+idB+"/messages?limit=0", cookieA), http.StatusBadRequest)
}
