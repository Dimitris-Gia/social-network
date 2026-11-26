package websocket

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"
)

// fakeClient registers a connection with no real socket behind it, enough
// to test which connections the hub closes.
func fakeClient(h *Hub, userID int64, sessionID string) *Client {
	c := &Client{hub: h, userID: userID, sessionID: sessionID, send: make(chan []byte, 1)}
	h.register(c)
	return c
}

func isClosed(c *Client) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func TestCloseSessionClosesOnlyThatSession(t *testing.T) {
	h := NewHub()
	tab1 := fakeClient(h, 1, "sess-a")
	tab2 := fakeClient(h, 1, "sess-a")  // same browser, second tab
	phone := fakeClient(h, 1, "sess-b") // same user, another login
	other := fakeClient(h, 2, "sess-c") // another user

	h.CloseSession("sess-a")

	if !isClosed(tab1) || !isClosed(tab2) {
		t.Error("both tabs of the logged-out session should be closed")
	}
	if isClosed(phone) {
		t.Error("the user's other session must stay connected")
	}
	if isClosed(other) {
		t.Error("another user's connection must stay open")
	}
	if !h.IsOnline(1) {
		t.Error("user 1 is still online through the other session")
	}
}

func TestCloseSessionIgnoresEmptyID(t *testing.T) {
	h := NewHub()
	c := fakeClient(h, 1, "")
	h.CloseSession("")
	if isClosed(c) {
		t.Error(`CloseSession("") must not close connections without a session`)
	}
}

func TestCloseInvalidSessions(t *testing.T) {
	h := NewHub()
	expired1 := fakeClient(h, 1, "expired")
	expired2 := fakeClient(h, 1, "expired")
	alive := fakeClient(h, 2, "alive")
	noSession := fakeClient(h, 3, "")

	// Before SetSessionCheck nothing happens.
	h.closeInvalidSessions()
	if isClosed(expired1) {
		t.Fatal("without a session check, nothing should be closed")
	}

	var mu sync.Mutex
	calls := map[string]int{}
	h.SetSessionCheck(func(id string) bool {
		mu.Lock()
		calls[id]++
		mu.Unlock()
		return id == "alive"
	})
	h.closeInvalidSessions()

	if !isClosed(expired1) || !isClosed(expired2) {
		t.Error("connections of an expired session should be closed")
	}
	if isClosed(alive) {
		t.Error("a valid session must stay connected")
	}
	if isClosed(noSession) {
		t.Error("a connection without a session ID is not checked")
	}
	if calls["expired"] != 1 {
		t.Errorf("expired session checked %d times, want 1 (once per session, not per tab)", calls["expired"])
	}
	if _, asked := calls[""]; asked {
		t.Error("an empty session ID must not be looked up")
	}
	if h.IsOnline(1) {
		t.Error("user 1 should be offline after their only session expired")
	}
}

func TestRunSessionCheckRunsPeriodically(t *testing.T) {
	h := NewHub()
	c := fakeClient(h, 1, "sess")
	h.SetSessionCheck(func(string) bool { return false })
	go h.RunSessionCheck(10 * time.Millisecond)
	waitFor(t, "the expired connection to be closed", func() bool { return isClosed(c) })
}

// End to end with a real socket: after CloseSession the browser sees the
// connection close (that's what makes the frontend reconnect, get a 401 and
// log out).
func TestCloseSessionClosesTheRealSocket(t *testing.T) {
	hub, srv := newTestServer(t)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/?user=" + strconv.Itoa(1) + "&session=sess-a"
	conn, _, err := gws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	waitFor(t, "user 1 online", func() bool { return hub.IsOnline(1) })

	hub.CloseSession("sess-a")

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Error("expected the server to close the connection")
	} else if strings.Contains(err.Error(), "timeout") {
		t.Errorf("connection was not closed, read timed out: %v", err)
	}
	waitFor(t, "user 1 offline", func() bool { return !hub.IsOnline(1) })
}
