// Owner: B
package websocket

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
)

// The test server stands in for the real route: it takes the user ID from
// ?user=N. (The real route gets it from RequireAuth, never from the URL.)
func newTestServer(t *testing.T) (*Hub, *httptest.Server) {
	t.Helper()
	hub := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.URL.Query().Get("user"), 10, 64)
		hub.ServeWS(w, r, id)
	}))
	t.Cleanup(srv.Close)
	return hub, srv
}

func dial(t *testing.T, srv *httptest.Server, userID int) *gws.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/?user=" + strconv.Itoa(userID)
	conn, _, err := gws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

type frame struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func readFrame(t *testing.T, conn *gws.Conn) frame {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var f frame
	if err := conn.ReadJSON(&f); err != nil {
		t.Fatalf("expected a frame: %v", err)
	}
	return f
}

// expectSilence must be the LAST read on a connection: after a timeout
// gorilla's connection is no longer usable.
func expectSilence(t *testing.T, conn *gws.Conn, who string) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	var f frame
	if err := conn.ReadJSON(&f); err == nil {
		t.Errorf("%s received an unexpected frame: %+v", who, f)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func TestSendToUserReachesOnlyThatUser(t *testing.T) {
	hub, srv := newTestServer(t)
	tab1 := dial(t, srv, 1)
	tab2 := dial(t, srv, 1) // same user, second tab
	other := dial(t, srv, 2)
	waitFor(t, "users online", func() bool { return hub.IsOnline(1) && hub.IsOnline(2) })
	waitFor(t, "both tabs registered", func() bool { return len(hub.connsOf(1)) == 2 })

	hub.SendToUser(1, Message{Type: "notification", Data: map[string]int{"id": 7}})

	for i, c := range []*gws.Conn{tab1, tab2} {
		f := readFrame(t, c)
		if f.Type != "notification" || string(f.Data) != `{"id":7}` {
			t.Errorf("tab %d got %+v", i+1, f)
		}
	}
	expectSilence(t, other, "user 2")
}

func TestDisconnectUnregisters(t *testing.T) {
	hub, srv := newTestServer(t)
	conn := dial(t, srv, 1)
	waitFor(t, "online", func() bool { return hub.IsOnline(1) })

	conn.Close()
	waitFor(t, "offline", func() bool { return !hub.IsOnline(1) })

	hub.SendToUser(1, Message{Type: "x"}) // must not panic or block
}

func TestHandleDispatchesWithTheRealUser(t *testing.T) {
	hub, srv := newTestServer(t)
	hub.Handle("echo", func(c *Client, data json.RawMessage) {
		c.Send(Message{Type: "echoed", Data: map[string]any{"user": c.UserID(), "data": data}})
	})
	conn := dial(t, srv, 42)

	// the frame claims to be from user 99: the hub must use the connection's user
	if err := conn.WriteJSON(map[string]any{"type": "echo", "data": map[string]any{"user_id": 99}}); err != nil {
		t.Fatal(err)
	}
	f := readFrame(t, conn)
	var got struct {
		User int64 `json:"user"`
	}
	json.Unmarshal(f.Data, &got)
	if f.Type != "echoed" || got.User != 42 {
		t.Errorf("got %+v (user %d), want echoed by user 42", f, got.User)
	}
}

func TestBadFramesGetAnErrorAndKeepTheConnection(t *testing.T) {
	hub, srv := newTestServer(t)
	hub.Handle("ping2", func(c *Client, _ json.RawMessage) { c.Send(Message{Type: "pong2"}) })
	conn := dial(t, srv, 1)

	for name, raw := range map[string]string{
		"not json":     `hello`,
		"no type":      `{"data": 1}`,
		"unknown type": `{"type": "nope", "data": 1}`,
	} {
		if err := conn.WriteMessage(gws.TextMessage, []byte(raw)); err != nil {
			t.Fatal(err)
		}
		if f := readFrame(t, conn); f.Type != "error" {
			t.Errorf("%s: got %+v, want an error frame", name, f)
		}
	}

	// still open and working
	conn.WriteJSON(map[string]any{"type": "ping2"})
	if f := readFrame(t, conn); f.Type != "pong2" {
		t.Errorf("connection stopped working after bad frames: %+v", f)
	}
}

func TestHandlerPanicDoesNotKillTheServer(t *testing.T) {
	hub, srv := newTestServer(t)
	hub.Handle("boom", func(*Client, json.RawMessage) { panic("bug") })
	hub.Handle("ok", func(c *Client, _ json.RawMessage) { c.Send(Message{Type: "fine"}) })
	conn := dial(t, srv, 1)

	conn.WriteJSON(map[string]any{"type": "boom"})
	if f := readFrame(t, conn); f.Type != "error" {
		t.Errorf("got %+v, want an error frame", f)
	}
	conn.WriteJSON(map[string]any{"type": "ok"})
	if f := readFrame(t, conn); f.Type != "fine" {
		t.Errorf("got %+v, want the connection to keep working", f)
	}
}

func TestOversizedFrameClosesTheConnection(t *testing.T) {
	_, srv := newTestServer(t)
	conn := dial(t, srv, 1)

	conn.WriteMessage(gws.TextMessage, []byte(strings.Repeat("a", maxMessageSize+1)))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Error("expected the server to close the connection")
	}
}

func TestSendToGroup(t *testing.T) {
	hub, srv := newTestServer(t)
	m1, m2, outsider := dial(t, srv, 1), dial(t, srv, 2), dial(t, srv, 3)
	waitFor(t, "all online", func() bool { return hub.IsOnline(1) && hub.IsOnline(2) && hub.IsOnline(3) })

	if err := hub.SendToGroup(7, Message{Type: "group_message"}); !errors.Is(err, ErrNoMembersFunc) {
		t.Errorf("without a lookup: got %v, want ErrNoMembersFunc", err)
	}

	hub.SetGroupMembers(func(groupID int64) ([]int64, error) {
		if groupID != 7 {
			return nil, errors.New("unknown group")
		}
		return []int64{1, 2, 2}, nil // a duplicate must not deliver twice
	})
	if err := hub.SendToGroup(7, Message{Type: "group_message", Data: "hi"}); err != nil {
		t.Fatal(err)
	}
	for i, c := range []*gws.Conn{m1, m2} {
		if f := readFrame(t, c); f.Type != "group_message" {
			t.Errorf("member %d got %+v", i+1, f)
		}
	}
	expectSilence(t, m2, "member 2 (second copy)")
	expectSilence(t, outsider, "non-member")

	if err := hub.SendToGroup(8, Message{Type: "x"}); err == nil {
		t.Error("a failing lookup must be returned as an error")
	}
}

// A browser that never reads must be dropped, not make the hub wait.
func TestSlowClientIsDropped(t *testing.T) {
	hub := NewHub()
	c := &Client{hub: hub, userID: 1, send: make(chan []byte, 1)} // no real connection
	hub.register(c)

	if !c.trySend([]byte("one")) {
		t.Fatal("first frame should fit in the buffer")
	}
	if c.trySend([]byte("two")) {
		t.Error("second frame should not fit")
	}
	if hub.IsOnline(1) {
		t.Error("a client with a full buffer must be unregistered")
	}
	if c.trySend([]byte("three")) { // after close: no panic, no delivery
		t.Error("a closed client must not accept frames")
	}
	hub.SendToUser(1, Message{Type: "x"}) // nobody left: must not panic
}
