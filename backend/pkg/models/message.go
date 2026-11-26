// Owner: B
// SaveMessage, History, CanChat, UnreadCount
//
// PRIVATE messages only (messages.receiver_id set, group_id NULL). C writes
// the group chat functions in their own file. Names here say "Private" or
// "Chat" so they cannot clash with C's.
//
// Who may chat (assignment): at least one of the two users follows the other
// (accepted follow). Every function below that reads or writes a
// conversation runs CanChat itself, so no handler can forget the check.

package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrChatNotAllowed: neither user follows the other (403 in API.md).
// An unknown user gives A's ErrUserNotFound (404).
var ErrChatNotAllowed = errors.New("chat not allowed: neither user follows the other")

// Page sizes for PrivateHistory (see API.md).
const (
	DefaultChatPageSize = 30
	MaxChatPageSize     = 100
)

// ChatMessage is the "Message" object of API.md.
type ChatMessage struct {
	ID         int64     `json:"id"`
	SenderID   int64     `json:"sender_id"`
	ReceiverID int64     `json:"receiver_id"`
	Content    string    `json:"content"`
	IsRead     bool      `json:"is_read"`
	CreatedAt  time.Time `json:"created_at"`
}

// ChatContact is one entry of "who can I chat with".
type ChatContact struct {
	User        PostAuthor   `json:"user"`
	LastMessage *ChatMessage `json:"last_message"` // nil if they never wrote to each other
	UnreadCount int          `json:"unread_count"`
}

// CanChat reports whether userID and otherID may exchange private messages:
// both exist, they are different people, and at least one follows the other.
// An unknown otherID gives ErrUserNotFound.
func CanChat(db *sql.DB, userID, otherID int64) (bool, error) {
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM users WHERE id = ?)`, otherID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check user: %w", err)
	}
	if !exists {
		return false, ErrUserNotFound
	}
	if userID == otherID {
		return false, nil
	}

	follows, err := IsFollowing(db, userID, otherID)
	if err != nil || follows {
		return follows, err
	}
	return IsFollowing(db, otherID, userID)
}

// requireChat turns CanChat into an error: nil, ErrUserNotFound or ErrChatNotAllowed.
func requireChat(db *sql.DB, userID, otherID int64) error {
	ok, err := CanChat(db, userID, otherID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrChatNotAllowed
	}
	return nil
}

const chatMessageColumns = `id, sender_id, receiver_id, content, is_read, created_at`

func scanChatMessage(r rowScanner) (ChatMessage, error) {
	var m ChatMessage
	err := r.Scan(&m.ID, &m.SenderID, &m.ReceiverID, &m.Content, &m.IsRead, &m.CreatedAt)
	return m, err
}

// SavePrivateMessage stores a message from senderID to receiverID and returns
// it. Returns ErrUserNotFound or ErrChatNotAllowed (nothing is stored).
func SavePrivateMessage(db *sql.DB, senderID, receiverID int64, content string) (*ChatMessage, error) {
	if err := requireChat(db, senderID, receiverID); err != nil {
		return nil, err
	}
	res, err := db.Exec(
		`INSERT INTO messages (sender_id, receiver_id, content) VALUES (?, ?, ?)`,
		senderID, receiverID, content,
	)
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("message id: %w", err)
	}
	m, err := scanChatMessage(db.QueryRow(`SELECT `+chatMessageColumns+` FROM messages WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("load message: %w", err)
	}
	return &m, nil
}

// PrivateHistory returns the conversation between viewerID and otherID, oldest
// first within the page. before is a message ID: only older messages are
// returned (0 = start from the newest). hasMore says whether older ones exist.
func PrivateHistory(db *sql.DB, viewerID, otherID int64, limit int, before int64) (messages []ChatMessage, hasMore bool, err error) {
	if err := requireChat(db, viewerID, otherID); err != nil {
		return nil, false, err
	}
	if limit <= 0 {
		limit = DefaultChatPageSize
	}
	if limit > MaxChatPageSize {
		limit = MaxChatPageSize
	}

	// ?1 = viewer, ?2 = other, ?3 = before, ?4 = limit (numbered parameters
	// may be reused). Newest first so LIMIT keeps the latest ones; reversed below.
	rows, err := db.Query(`
		SELECT `+chatMessageColumns+`
		FROM messages
		WHERE group_id IS NULL
		  AND ((sender_id = ?1 AND receiver_id = ?2) OR (sender_id = ?2 AND receiver_id = ?1))
		  AND (?3 = 0 OR id < ?3)
		ORDER BY id DESC
		LIMIT ?4`,
		viewerID, otherID, before, limit+1, // one extra row tells us whether older messages exist
	)
	if err != nil {
		return nil, false, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	messages = []ChatMessage{} // empty slice, not nil, so JSON is [] and not null
	for rows.Next() {
		m, err := scanChatMessage(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list messages: %w", err)
	}

	hasMore = len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 { // oldest first
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, hasMore, nil
}

// MarkConversationRead marks every message from otherID to readerID as read.
func MarkConversationRead(db *sql.DB, readerID, otherID int64) error {
	if err := requireChat(db, readerID, otherID); err != nil {
		return err
	}
	_, err := db.Exec(
		`UPDATE messages SET is_read = 1
		 WHERE receiver_id = ? AND sender_id = ? AND is_read = 0`,
		readerID, otherID,
	)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	return nil
}

// UnreadPrivateCount is the number behind the chat badge. It counts private
// messages only (group messages have no receiver), and has nothing to do with
// notifications.
func UnreadPrivateCount(db *sql.DB, userID int64) (int, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM messages WHERE receiver_id = ? AND is_read = 0`, userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("unread count: %w", err)
	}
	return n, nil
}

// ChatContacts lists the users userID may chat with (at least one follows the
// other), the most recent conversation first, then by name. Users with whom
// there is no conversation yet come last.
func ChatContacts(db *sql.DB, userID int64) ([]ChatContact, error) {
	// ?1 = the logged-in user. The join picks each contact's newest private message.
	rows, err := db.Query(`
		SELECT u.id, u.first_name, u.last_name, u.nickname, u.avatar_path,
		       m.id, m.sender_id, m.receiver_id, m.content, m.is_read, m.created_at,
		       (SELECT COUNT(*) FROM messages x
		         WHERE x.sender_id = u.id AND x.receiver_id = ?1 AND x.is_read = 0)
		FROM users u
		LEFT JOIN messages m ON m.id = (
		        SELECT MAX(id) FROM messages
		        WHERE group_id IS NULL
		          AND ((sender_id = ?1 AND receiver_id = u.id) OR (sender_id = u.id AND receiver_id = ?1)))
		WHERE u.id != ?1
		  AND EXISTS (
		        SELECT 1 FROM follows f
		        WHERE f.status = 'accepted'
		          AND ((f.follower_id = ?1 AND f.following_id = u.id)
		            OR (f.follower_id = u.id AND f.following_id = ?1)))
		ORDER BY m.id IS NULL, m.id DESC, u.first_name, u.id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	contacts := []ChatContact{} // [] not null in JSON
	for rows.Next() {
		var (
			c                       ChatContact
			mID, mSender, mReceiver sql.NullInt64
			mContent                sql.NullString
			mRead                   sql.NullBool
			mCreated                sql.NullTime
		)
		if err := rows.Scan(
			&c.User.ID, &c.User.FirstName, &c.User.LastName, &c.User.Nickname, &c.User.AvatarPath,
			&mID, &mSender, &mReceiver, &mContent, &mRead, &mCreated,
			&c.UnreadCount,
		); err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		if mID.Valid {
			c.LastMessage = &ChatMessage{
				ID: mID.Int64, SenderID: mSender.Int64, ReceiverID: mReceiver.Int64,
				Content: mContent.String, IsRead: mRead.Bool, CreatedAt: mCreated.Time,
			}
		}
		contacts = append(contacts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	return contacts, nil
}
