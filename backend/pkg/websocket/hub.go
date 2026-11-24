// Owner: B
// map userID -> clients; SendToUser, SendToGroup
//
// The hub moves JSON frames between the server and the browsers. It knows
// nothing about chat, notifications or groups: features plug in from outside.
//
// Every frame, in both directions, is {"type": "...", "data": ...} (API.md).
//
// How other owners use it (all calls are safe from any goroutine):
//
//	hub.SendToUser(userID, websocket.Message{Type: "notification", Data: n})   // A
//	hub.SendToGroup(groupID, websocket.Message{Type: "group_message", ...})   // C
//
//	// React to a frame sent BY a browser (register once, at start-up):
//	hub.Handle("group_message", func(c *websocket.Client, data json.RawMessage) {
//	    // c.UserID() is the logged-in sender. NEVER trust a user ID in data.
//	    // Check permissions here, then reply with c.Send / c.SendError.
//	})
//
// A user can have several connections (several tabs): every send goes to all
// of them. Handlers run on the connection's read goroutine, so keep them
// short and do not block on slow work.

package websocket

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
)

// Message is one frame sent TO a browser.
type Message struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// HandlerFunc handles one frame sent BY a browser. data is the raw "data"
// value of the frame; decode it with json.Unmarshal.
type HandlerFunc func(c *Client, data json.RawMessage)

// MembersFunc lists the user IDs that belong to a group. C provides it, so
// the hub does not need to know the groups tables.
type MembersFunc func(groupID int64) ([]int64, error)

// ErrNoMembersFunc is returned by SendToGroup until SetGroupMembers is called.
var ErrNoMembersFunc = errors.New("websocket: group members lookup not set")

type Hub struct {
	mu       sync.RWMutex
	clients  map[int64]map[*Client]struct{} // userID -> that user's connections
	handlers map[string]HandlerFunc         // frame type -> handler
	members  MembersFunc
}

func NewHub() *Hub {
	return &Hub{
		clients:  make(map[int64]map[*Client]struct{}),
		handlers: make(map[string]HandlerFunc),
	}
}

// Handle registers fn for frames of the given type. Call it at start-up,
// before the first connection. A second call for the same type replaces the first.
func (h *Hub) Handle(msgType string, fn HandlerFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[msgType] = fn
}

// SetGroupMembers tells the hub how to find the members of a group (C).
func (h *Hub) SetGroupMembers(fn MembersFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.members = fn
}

func (h *Hub) handlerFor(msgType string) (HandlerFunc, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	fn, ok := h.handlers[msgType]
	return fn, ok
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.userID] == nil {
		h.clients[c.userID] = make(map[*Client]struct{})
	}
	h.clients[c.userID][c] = struct{}{}
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns := h.clients[c.userID]; conns != nil {
		delete(conns, c)
		if len(conns) == 0 {
			delete(h.clients, c.userID)
		}
	}
}

// IsOnline reports whether the user has at least one open connection.
func (h *Hub) IsOnline(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// connsOf returns a copy of the user's connections, so callers can send
// without holding the hub's lock (a failed send unregisters, which locks).
func (h *Hub) connsOf(userID int64) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Client, 0, len(h.clients[userID]))
	for c := range h.clients[userID] {
		out = append(out, c)
	}
	return out
}

// SendToUser delivers msg to every open connection of the user. A user who
// is offline is skipped silently: the data is in the database anyway.
func (h *Hub) SendToUser(userID int64, msg Message) {
	h.SendToUsers([]int64{userID}, msg)
}

// SendToUsers delivers msg to every connection of every listed user, once
// per user even if an ID is listed twice.
func (h *Hub) SendToUsers(userIDs []int64, msg Message) {
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("websocket: cannot marshal %q frame: %v", msg.Type, err)
		return
	}
	seen := make(map[int64]bool, len(userIDs))
	for _, id := range userIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, c := range h.connsOf(id) {
			c.trySend(payload)
		}
	}
}

// SendToGroup delivers msg to every online member of the group. The member
// list comes from the function set with SetGroupMembers.
func (h *Hub) SendToGroup(groupID int64, msg Message) error {
	h.mu.RLock()
	lookup := h.members
	h.mu.RUnlock()
	if lookup == nil {
		return ErrNoMembersFunc
	}
	ids, err := lookup(groupID)
	if err != nil {
		return err
	}
	h.SendToUsers(ids, msg)
	return nil
}
