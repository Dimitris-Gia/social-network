// Owner: C
// Group chat messages and history. These are separate from private chat.

package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrGroupChatNotAllowed = errors.New("group chat not allowed: user is not a member")
	ErrGroupNotFound       = errors.New("group not found")
)

const (
	DefaultGroupChatPageSize = 30
	MaxGroupChatPageSize     = 100
)

// GroupChatMessage is a message saved in one group's room.
type GroupChatMessage struct {
	ID        int64     `json:"id"`
	SenderID  int64     `json:"sender_id"`
	GroupID   int64     `json:"group_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func requireGroupMember(db *sql.DB, userID, groupID int64) error {
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM groups WHERE id = ?)`, groupID).Scan(&exists); err != nil {
		return fmt.Errorf("check group: %w", err)
	}
	if !exists {
		return ErrGroupNotFound
	}

	isMember, err := IsMember(db, userID, groupID)
	if err != nil {
		return fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return ErrGroupChatNotAllowed
	}
	return nil
}

// SaveGroupMessage validates membership and stores a group chat message.
func SaveGroupMessage(db *sql.DB, senderID, groupID int64, content string) (*GroupChatMessage, error) {
	if err := requireGroupMember(db, senderID, groupID); err != nil {
		return nil, err
	}

	res, err := db.Exec(`INSERT INTO messages (sender_id, group_id, content) VALUES (?, ?, ?)`, senderID, groupID, content)
	if err != nil {
		return nil, fmt.Errorf("insert group message: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("group message id: %w", err)
	}

	var message GroupChatMessage
	err = db.QueryRow(`SELECT id, sender_id, group_id, content, created_at
		FROM messages WHERE id = ? AND receiver_id IS NULL AND group_id IS NOT NULL`, id).
		Scan(&message.ID, &message.SenderID, &message.GroupID, &message.Content, &message.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("load group message: %w", err)
	}
	return &message, nil
}

// GroupHistory returns oldest-first messages visible to a confirmed member.
func GroupHistory(db *sql.DB, viewerID, groupID int64, limit int, before int64) ([]GroupChatMessage, bool, error) {
	if err := requireGroupMember(db, viewerID, groupID); err != nil {
		return nil, false, err
	}
	if limit <= 0 {
		limit = DefaultGroupChatPageSize
	}
	if limit > MaxGroupChatPageSize {
		limit = MaxGroupChatPageSize
	}

	rows, err := db.Query(`
		SELECT id, sender_id, group_id, content, created_at
		FROM messages
		WHERE receiver_id IS NULL AND group_id = ?
		  AND (? = 0 OR id < ?)
		ORDER BY id DESC
		LIMIT ?`, groupID, before, before, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("list group messages: %w", err)
	}
	defer rows.Close()

	messages := make([]GroupChatMessage, 0)
	for rows.Next() {
		var message GroupChatMessage
		if err := rows.Scan(&message.ID, &message.SenderID, &message.GroupID, &message.Content, &message.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("scan group message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate group messages: %w", err)
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, hasMore, nil
}
