// Owner: B
// readPump / writePump per connection (gorilla/websocket)
//
// One Client is one browser connection. Two goroutines serve it:
//   - readPump reads frames from the browser and hands them to the hub's handlers;
//   - writePump is the ONLY place that writes to the connection (gorilla allows
//     one writer at a time), taking frames from the send channel.

package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	gws "github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second  // max time to write one frame
	pongWait       = 60 * time.Second  // the browser must answer our pings within this
	pingPeriod     = pongWait * 9 / 10 // must be shorter than pongWait
	maxMessageSize = 16 * 1024         // largest frame we accept from a browser
	sendBuffer     = 256               // frames queued for one slow browser
)

// The default CheckOrigin only accepts a page from the same host, which
// blocks other websites from opening a socket with the user's cookie.
var upgrader = gws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Client struct {
	hub    *Hub
	conn   *gws.Conn // nil only in unit tests
	userID int64
	// sessionID is the session that opened this connection. Logout and the
	// expiry check use it to close the connection when the session ends
	// (the session is only checked once, at the upgrade, otherwise).
	sessionID string
	send      chan []byte

	mu     sync.Mutex // protects closed and the closing of send
	closed bool
}

// ServeWS upgrades the HTTP request to a WebSocket for userID and serves it
// until the browser leaves, or until the session ends (see CloseSession and
// RunSessionCheck). The caller must already have authenticated the user
// (RequireAuth does it for GET /ws): never pass a user ID from the request.
// sessionID is the cookie that RequireAuth accepted.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, userID int64, sessionID string) {
	conn, err := upgrader.Upgrade(w, r, nil) // on failure it has already answered
	if err != nil {
		log.Printf("websocket: upgrade for user %d failed: %v", userID, err)
		return
	}
	c := &Client{hub: h, conn: conn, userID: userID, sessionID: sessionID, send: make(chan []byte, sendBuffer)}
	h.register(c)
	go c.writePump()
	go c.readPump()
}

// UserID is the logged-in user who owns this connection.
func (c *Client) UserID() int64 { return c.userID }

// Send queues a frame for THIS connection only (to reach all of a user's
// connections, use hub.SendToUser).
func (c *Client) Send(msg Message) {
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("websocket: cannot marshal %q frame: %v", msg.Type, err)
		return
	}
	c.trySend(payload)
}

// SendError queues {"type":"error","data":{"message":text}} for this connection.
// The connection stays open.
func (c *Client) SendError(text string) {
	c.Send(Message{Type: "error", Data: map[string]string{"message": text}})
}

// trySend queues payload without ever blocking. If the browser is so slow
// that its buffer is full, the connection is dropped instead of making the
// whole hub wait. It reports whether the frame was queued.
func (c *Client) trySend(payload []byte) bool {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	select {
	case c.send <- payload:
		c.mu.Unlock()
		return true
	default:
		c.mu.Unlock()
		log.Printf("websocket: user %d is too slow, dropping connection", c.userID)
		c.close()
		return false
	}
}

// close stops the client exactly once: it leaves the hub and closes send,
// which tells writePump to say goodbye and close the connection.
func (c *Client) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.send)
	c.mu.Unlock()
	c.hub.unregister(c)
}

func (c *Client) readPump() {
	defer c.close()
	c.conn.SetReadLimit(maxMessageSize) // a bigger frame closes the connection
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return // browser left, timed out, or sent an oversized frame
		}

		var in struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &in); err != nil || in.Type == "" {
			c.SendError("invalid message: expected {\"type\": ..., \"data\": ...}")
			continue
		}
		fn, ok := c.hub.handlerFor(in.Type)
		if !ok {
			c.SendError("unknown message type: " + in.Type)
			continue
		}
		c.dispatch(fn, in.Data)
	}
}

// dispatch runs a handler and survives its panics: one buggy handler must
// not take the whole server down.
func (c *Client) dispatch(fn HandlerFunc, data json.RawMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("websocket: handler panic for user %d: %v", c.userID, r)
			c.SendError("internal server error")
		}
	}()
	fn(c, data)
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer c.conn.Close()

	for {
		select {
		case payload, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok { // close() was called
				c.conn.WriteMessage(gws.CloseMessage, gws.FormatCloseMessage(gws.CloseNormalClosure, ""))
				return
			}
			if err := c.conn.WriteMessage(gws.TextMessage, payload); err != nil {
				c.close()
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(gws.PingMessage, nil); err != nil {
				c.close()
				return
			}
		}
	}
}
