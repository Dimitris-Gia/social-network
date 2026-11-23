// Owner: A
// CreateSession, GetUserIDBySession, DeleteSession

package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// SessionDuration is how long a login lasts. The cookie's Max-Age uses the
// same value, so the browser and the database forget the session together.
const SessionDuration = 7 * 24 * time.Hour

// ErrNoSession means the session ID is unknown or has expired. The middleware
// treats both the same way: 401, please log in again.
var ErrNoSession = errors.New("no valid session")

// sqliteTime is the format SQLite's own datetime('now') returns. We store
// expires_at in exactly this format, in UTC, so the expiry check can be done
// inside the SQL query by comparing two strings of the same shape.
const sqliteTime = "2006-01-02 15:04:05"

// CreateSession stores a new session for userID and returns its ID, which
// becomes the cookie value.
//
// The ID is a random (version 4) UUID: 122 random bits from crypto/rand, so it
// can't be guessed. It is NOT derived from the user ID or the time, which
// would make it predictable.
func CreateSession(db *sql.DB, userID int64) (string, error) {
	id := uuid.NewString()
	expires := time.Now().UTC().Add(SessionDuration).Format(sqliteTime)
	_, err := db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		id, userID, expires)
	if err != nil {
		return "", err
	}
	return id, nil
}

// GetUserIDBySession returns the user who owns a session that has not expired.
// An expired row is treated exactly like a missing one (ErrNoSession), so a
// cookie someone copied stops working after 7 days even if the browser kept it.
func GetUserIDBySession(db *sql.DB, sessionID string) (int64, error) {
	var userID int64
	err := db.QueryRow(`SELECT user_id FROM sessions
		WHERE id = ? AND expires_at > datetime('now')`, sessionID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNoSession
	}
	return userID, err
}

// DeleteSession removes one session (logout, or replacing an old session on
// login). Deleting an ID that doesn't exist is not an error.
func DeleteSession(db *sql.DB, sessionID string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id = ?`, sessionID)
	return err
}

// DeleteExpiredSessions removes a user's dead sessions so the table doesn't
// grow forever. Called on login; cheap thanks to the index on user_id.
func DeleteExpiredSessions(db *sql.DB, userID int64) error {
	_, err := db.Exec(`DELETE FROM sessions
		WHERE user_id = ? AND expires_at <= datetime('now')`, userID)
	return err
}
