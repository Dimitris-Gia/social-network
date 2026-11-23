package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/mattn/go-sqlite3"
)

// ErrEmailTaken is returned by CreateUser when the UNIQUE constraint on
// users.email rejects the insert.
var ErrEmailTaken = errors.New("email already registered")

// User is one row of the users table plus a few fields that describe how the
// *viewer* relates to this user (counts, follow_status, can_view). Those extra
// fields are filled in by the handler, not read from the users table.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // never sent to the client, whatever happens
	FirstName    string    `json:"first_name"`
	LastName     string    `json:"last_name"`
	DateOfBirth  string    `json:"date_of_birth"` // "YYYY-MM-DD"
	Nickname     *string   `json:"nickname"`      // nil -> JSON null
	AboutMe      *string   `json:"about_me"`
	Avatar       *string   `json:"avatar"` // URL path, e.g. "/uploads/<uuid>.png"
	IsPrivate    bool      `json:"is_private"`
	CreatedAt    time.Time `json:"created_at"`

	FollowersCount int    `json:"followers_count"`
	FollowingCount int    `json:"following_count"`
	FollowStatus   string `json:"follow_status"` // self | none | pending | following
	CanView        bool   `json:"can_view"`
}

// userColumns is the column list every SELECT on users uses, in the order
// scanUser expects. Keeping it in one place means the two can't drift apart.
const userColumns = `id, email, password_hash, first_name, last_name, date_of_birth,
	nickname, about_me, avatar_path, is_private, created_at`

// scanUser works with both *sql.Row and *sql.Rows.
func scanUser(s interface{ Scan(...any) error }) (*User, error) {
	var u User
	err := s.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FirstName, &u.LastName,
		&u.DateOfBirth, &u.Nickname, &u.AboutMe, &u.Avatar, &u.IsPrivate, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// EmailExists reports whether an account already uses this email.
// The email must already be normalised (trimmed + lowercased) by the caller.
func EmailExists(db *sql.DB, email string) (bool, error) {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)`, email).Scan(&exists)
	return exists, err
}

// CreateUser inserts u and then reloads it, so u gets its ID, created_at and
// the default is_private from the database.
//
// Values go in through ? placeholders: the driver sends them separately from
// the SQL text, so input like  ' OR 1=1 --  is stored as plain text and can
// never change the query (no SQL injection).
func CreateUser(db *sql.DB, u *User) error {
	res, err := db.Exec(`INSERT INTO users
		(email, password_hash, first_name, last_name, date_of_birth, nickname, about_me, avatar_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Email, u.PasswordHash, u.FirstName, u.LastName, u.DateOfBirth,
		u.Nickname, u.AboutMe, u.Avatar) // nil *string is stored as NULL
	if err != nil {
		// Two people registering the same email at the same moment can both
		// pass EmailExists; the UNIQUE constraint is the guard that always wins.
		var sqlErr sqlite3.Error
		if errors.As(err, &sqlErr) && sqlErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return ErrEmailTaken
		}
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	created, err := GetUserByID(db, id)
	if err != nil {
		return err
	}
	*u = *created
	return nil
}

// GetUserByID returns sql.ErrNoRows if there is no such user.
func GetUserByID(db *sql.DB, id int64) (*User, error) {
	return scanUser(db.QueryRow(`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

// GetUserByEmail returns sql.ErrNoRows if there is no such user.
// Used by login (Wednesday).
func GetUserByEmail(db *sql.DB, email string) (*User, error) {
	return scanUser(db.QueryRow(`SELECT `+userColumns+` FROM users WHERE email = ?`, email))
}
