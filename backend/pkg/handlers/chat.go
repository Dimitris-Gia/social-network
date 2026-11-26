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
	h.Hub.Handle("group_message", h.onGroupMessage)
	h.Hub.SetGroupMembers(func(groupID int64) ([]int64, error) {
		return models.ConfirmedMemberIDs(h.DB, groupID)
	})
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

// onGroupMessage handles {"type":"group_message","data":{"group_id":3,"content":"Hi"}}.
func (h *Handler) onGroupMessage(c *websocket.Client, data json.RawMessage) {
	var in struct {
		GroupID int64  `json:"group_id"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &in); err != nil || in.GroupID < 1 {
		c.SendError(`group_message needs {"group_id": <group id>, "content": <text>}`)
		return
	}

	content := strings.TrimSpace(in.Content)
	if n := utf8.RuneCountInString(content); n == 0 || n > maxChatLength {
		c.SendError("content must be 1 to 2000 characters")
		return
	}

	senderID := c.UserID()
	message, err := models.SaveGroupMessage(h.DB, senderID, in.GroupID, content)
	switch {
	case errors.Is(err, models.ErrGroupNotFound):
		c.SendError("group not found")
		return
	case errors.Is(err, models.ErrGroupChatNotAllowed):
		log.Printf("group chat: user %d tried to message group %d but is not a member", senderID, in.GroupID)
		c.SendError("you must be a confirmed group member to chat")
		return
	case err != nil:
		log.Printf("group chat: save message from %d to group %d: %v", senderID, in.GroupID, err)
		c.SendError("internal server error")
		return
	}

	if err := h.Hub.SendToGroup(in.GroupID, websocket.Message{Type: "group_message", Data: message}); err != nil {
		log.Printf("group chat: send message to group %d: %v", in.GroupID, err)
	}
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

// GroupChatHistory handles GET /api/groups/{id}/messages.
func (h *Handler) GroupChatHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r)
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	groupID, err := groupIDFromRequest(r)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	limit, before, valid := postPageParams(w, r)
	if !valid {
		return
	}

	messages, hasMore, err := models.GroupHistory(h.DB, userID, groupID, limit, before)
	switch {
	case errors.Is(err, models.ErrGroupNotFound):
		utils.WriteError(w, http.StatusNotFound, "group not found")
	case errors.Is(err, models.ErrGroupChatNotAllowed):
		utils.WriteError(w, http.StatusForbidden, "you must be a confirmed group member to chat")
	case err != nil:
		utils.ServerError(w, err)
	default:
		utils.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages, "has_more": hasMore})
	}
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
