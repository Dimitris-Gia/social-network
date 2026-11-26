// Owner: B
// GET /ws upgrade, history, "who can I chat with"
//
// REST: contacts, history, mark as read, unread badge.
// WebSocket: the "private_message" frame, plugged into the hub.
// The permission rule (at least one user follows the other) lives in
// models.CanChat; SavePrivateMessage and the history functions run it
// themselves, so nothing here can skip it.

package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"social-network/pkg/middleware"
	"social-network/pkg/models"
	"social-network/pkg/utils"
	"social-network/pkg/websocket"
)

const maxChatLength = 2000 // characters (API.md)

// ServeWS handles GET /ws: it upgrades the request to a WebSocket and keeps
// it open. The route is registered with protected(...), so RequireAuth has
// already checked the session cookie (no cookie: 401 and no upgrade) and the
// user ID comes from the session, never from the URL or from a frame.
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	// RequireAuth already accepted this cookie. The hub keeps it so it can
	// close the socket on logout or when the session expires.
	cookie, _ := r.Cookie(middleware.CookieName)
	h.Hub.ServeWS(w, r, userID, cookie.Value)
}

// registerChatHandlers tells the hub what to do with frames sent by browsers.
// RegisterRoutes calls it once at start-up.
func (h *Handler) registerChatHandlers() {
	if h.Hub == nil {
		return
	}
	h.Hub.Handle("private_message", h.onPrivateMessage)
}

// onPrivateMessage handles {"type":"private_message","data":{"to":5,"content":"Hi"}}.
//
// The sender is ALWAYS the connection's user (c.UserID()). Anything else in
// the frame, such as a "sender_id", is ignored. A refused message is logged and
// answered with an error frame; the connection stays open.
func (h *Handler) onPrivateMessage(c *websocket.Client, data json.RawMessage) {
	var in struct {
		To      int64  `json:"to"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &in); err != nil || in.To < 1 {
		c.SendError(`private_message needs {"to": <user id>, "content": <text>}`)
		return
	}
	content := strings.TrimSpace(in.Content)
	if n := utf8.RuneCountInString(content); n == 0 || n > maxChatLength {
		c.SendError("content must be 1 to 2000 characters")
		return
	}

	senderID := c.UserID()
	msg, err := models.SavePrivateMessage(h.DB, senderID, in.To, content)
	switch {
	case errors.Is(err, models.ErrUserNotFound):
		c.SendError("user not found")
		return
	case errors.Is(err, models.ErrChatNotAllowed):
		log.Printf("chat: user %d tried to message user %d but neither follows the other: refused", senderID, in.To)
		c.SendError("you can only chat with users you follow or who follow you")
		return
	case err != nil:
		log.Printf("chat: save message from %d to %d: %v", senderID, in.To, err)
		c.SendError("internal server error")
		return
	}

	// The recipient gets it instantly, and so do all of the sender's own tabs.
	frame := websocket.Message{Type: "private_message", Data: msg}
	h.Hub.SendToUser(in.To, frame)
	h.Hub.SendToUser(senderID, frame)
}

// chatError answers for a failed chat model call. It reports whether err was
// set, so the caller can return.
func chatError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, models.ErrUserNotFound):
		utils.WriteError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, models.ErrChatNotAllowed):
		utils.WriteError(w, http.StatusForbidden, "you can only chat with users you follow or who follow you")
	default:
		utils.ServerError(w, err)
	}
	return true
}

// ChatContacts handles GET /api/chat/contacts.
func (h *Handler) ChatContacts(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	contacts, err := models.ChatContacts(h.DB, userID)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]any{"contacts": contacts})
}

// ChatHistory handles GET /api/chat/{id}/messages?limit=&before=.
func (h *Handler) ChatHistory(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	otherID, ok := postPathID(w, r, "user")
	if !ok {
		return
	}
	limit, before, ok := postPageParams(w, r)
	if !ok {
		return
	}

	messages, hasMore, err := models.PrivateHistory(h.DB, userID, otherID, limit, before)
	if chatError(w, err) {
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages, "has_more": hasMore})
}

// ChatMarkRead handles POST /api/chat/{id}/read.
func (h *Handler) ChatMarkRead(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	otherID, ok := postPathID(w, r, "user")
	if !ok {
		return
	}
	if chatError(w, models.MarkConversationRead(h.DB, userID, otherID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ChatUnreadCount handles GET /api/chat/unread-count (the chat badge, which
// is separate from the notification bell).
func (h *Handler) ChatUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	n, err := models.UnreadPrivateCount(h.DB, userID)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]int{"unread_count": n})
}
