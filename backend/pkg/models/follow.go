// Owner: A
// Follow, Accept, Delete, IsFollowing, Followers, Following
//
// One row in `follows` = "follower_id follows following_id".
//   status 'pending'  -> a request to a private profile, not answered yet
//   status 'accepted' -> a real follow
// The primary key (follower_id, following_id) means there can only ever be
// ONE row per pair, so "the request from user 3 to me" needs no extra ID.

package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/mattn/go-sqlite3"
)

// Values of follow_status sent to the frontend (API.md, part 1).
// "pending"/"accepted" are what's stored in the DB; these are what the
// Follow button needs, so the conversion happens here, in one place.
const (
	FollowSelf      = "self"
	FollowNone      = "none"
	FollowPending   = "pending"
	FollowFollowing = "following"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrAlreadyFollowing = errors.New("already following or already requested")
	ErrNotFollowing     = errors.New("you don't follow this user")
	ErrNoFollowRequest  = errors.New("no pending follow request from this user")
)

// UserSummary is the small user object used in lists (API.md "UserSummary").
// It never contains the email, date of birth or about_me, so lists can't leak
// a private profile's details.
type UserSummary struct {
	ID        int64   `json:"id"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	Nickname  *string `json:"nickname"`
	Avatar    *string `json:"avatar"`
}

// FollowRequest is one pending request TO the logged-in user.
type FollowRequest struct {
	User      UserSummary `json:"user"`
	CreatedAt time.Time   `json:"created_at"`
}

// summaryColumns must match the Scan order in scanSummary.
// "u" is the alias every query below gives the users table.
const summaryColumns = `u.id, u.first_name, u.last_name, u.nickname, u.avatar_path`

func scanSummary(s interface{ Scan(...any) error }, extra ...any) (UserSummary, error) {
	var u UserSummary
	dest := append([]any{&u.ID, &u.FirstName, &u.LastName, &u.Nickname, &u.Avatar}, extra...)
	err := s.Scan(dest...)
	return u, err
}

// GetFollowStatus tells how viewerID relates to targetID:
// self, none, pending or following.
func GetFollowStatus(db *sql.DB, viewerID, targetID int64) (string, error) {
	if viewerID == targetID {
		return FollowSelf, nil
	}
	var status string
	err := db.QueryRow(`SELECT status FROM follows WHERE follower_id = ? AND following_id = ?`,
		viewerID, targetID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return FollowNone, nil
	}
	if err != nil {
		return "", err
	}
	if status == "accepted" {
		return FollowFollowing, nil
	}
	return FollowPending, nil
}

// IsFollowing reports whether followerID has an ACCEPTED follow on
// followingID. A pending request does not count: it gives no access to
// anything. B's postIsFollowing (post.go) can be replaced by this.
func IsFollowing(db *sql.DB, followerID, followingID int64) (bool, error) {
	var ok bool
	err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM follows
		WHERE follower_id = ? AND following_id = ? AND status = 'accepted')`,
		followerID, followingID).Scan(&ok)
	return ok, err
}

// CanViewProfile is THE private-profile gate (API.md 1.2). Every endpoint
// that shows a profile's details, follower lists or posts goes through it:
// you may see it if it's you, the profile is public, or you follow it
// (accepted). Returns ErrUserNotFound for an unknown ID.
func CanViewProfile(db *sql.DB, viewerID, targetID int64) (bool, error) {
	var private bool
	err := db.QueryRow(`SELECT is_private FROM users WHERE id = ?`, targetID).Scan(&private)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrUserNotFound
	}
	if err != nil {
		return false, err
	}
	if viewerID == targetID || !private {
		return true, nil
	}
	return IsFollowing(db, viewerID, targetID)
}

// Follow creates the follow row and returns the new follow_status:
//   - public target  -> stored 'accepted', returns "following" (instant)
//   - private target -> stored 'pending',  returns "pending" (needs accept)
//
// The caller rejects following yourself; the CHECK in the table is a second guard.
func Follow(db *sql.DB, followerID, followingID int64) (string, error) {
	var private bool
	err := db.QueryRow(`SELECT is_private FROM users WHERE id = ?`, followingID).Scan(&private)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", err
	}

	dbStatus, status := "accepted", FollowFollowing
	if private {
		dbStatus, status = "pending", FollowPending
	}

	_, err = db.Exec(`INSERT INTO follows (follower_id, following_id, status) VALUES (?, ?, ?)`,
		followerID, followingID, dbStatus)
	if err != nil {
		// The primary key already holds a row for this pair: either a real
		// follow or a request that's still pending.
		var sqlErr sqlite3.Error
		if errors.As(err, &sqlErr) && sqlErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
			return "", ErrAlreadyFollowing
		}
		return "", err
	}
	return status, nil
}

// Unfollow deletes the row whatever its status, so the same call is
// "unfollow" for an accepted follow and "cancel my request" for a pending one.
func Unfollow(db *sql.DB, followerID, followingID int64) error {
	res, err := db.Exec(`DELETE FROM follows WHERE follower_id = ? AND following_id = ?`,
		followerID, followingID)
	if err != nil {
		return err
	}
	return mustAffectOne(res, ErrNotFollowing)
}

// AcceptFollowRequest turns requesterID's pending request to userID into a
// real follow. userID is ALWAYS the logged-in user: because it's in the
// WHERE clause, nobody can accept a request that was sent to someone else.
func AcceptFollowRequest(db *sql.DB, userID, requesterID int64) error {
	res, err := db.Exec(`UPDATE follows SET status = 'accepted'
		WHERE follower_id = ? AND following_id = ? AND status = 'pending'`,
		requesterID, userID)
	if err != nil {
		return err
	}
	return mustAffectOne(res, ErrNoFollowRequest)
}

// DeclineFollowRequest deletes the pending request. Same WHERE rule as accept.
// "AND status = 'pending'" means decline can't be used to remove a follower
// who was already accepted.
func DeclineFollowRequest(db *sql.DB, userID, requesterID int64) error {
	res, err := db.Exec(`DELETE FROM follows
		WHERE follower_id = ? AND following_id = ? AND status = 'pending'`,
		requesterID, userID)
	if err != nil {
		return err
	}
	return mustAffectOne(res, ErrNoFollowRequest)
}

// PendingFollowRequests lists the requests sent TO userID, newest first.
func PendingFollowRequests(db *sql.DB, userID int64) ([]FollowRequest, error) {
	rows, err := db.Query(`SELECT `+summaryColumns+`, f.created_at
		FROM follows f JOIN users u ON u.id = f.follower_id
		WHERE f.following_id = ? AND f.status = 'pending'
		ORDER BY f.created_at DESC, f.rowid DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []FollowRequest{} // [] in JSON, not null, when empty
	for rows.Next() {
		var req FollowRequest
		req.User, err = scanSummary(rows, &req.CreatedAt)
		if err != nil {
			return nil, err
		}
		list = append(list, req)
	}
	return list, rows.Err()
}

// Followers lists the users who follow userID (accepted only), newest first.
func Followers(db *sql.DB, userID int64) ([]UserSummary, error) {
	return listSummaries(db, `SELECT `+summaryColumns+`
		FROM follows f JOIN users u ON u.id = f.follower_id
		WHERE f.following_id = ? AND f.status = 'accepted'
		ORDER BY f.created_at DESC, f.rowid DESC`, userID)
}

// Following lists the users userID follows (accepted only), newest first.
func Following(db *sql.DB, userID int64) ([]UserSummary, error) {
	return listSummaries(db, `SELECT `+summaryColumns+`
		FROM follows f JOIN users u ON u.id = f.following_id
		WHERE f.follower_id = ? AND f.status = 'accepted'
		ORDER BY f.created_at DESC, f.rowid DESC`, userID)
}

// FollowCounts returns how many accepted followers and followings userID has.
func FollowCounts(db *sql.DB, userID int64) (followers, following int, err error) {
	err = db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM follows WHERE following_id = ? AND status = 'accepted'),
		(SELECT COUNT(*) FROM follows WHERE follower_id  = ? AND status = 'accepted')`,
		userID, userID).Scan(&followers, &following)
	return followers, following, err
}

// listSummaries runs a query whose columns are summaryColumns.
func listSummaries(db *sql.DB, query string, args ...any) ([]UserSummary, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []UserSummary{}
	for rows.Next() {
		u, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// mustAffectOne turns "the UPDATE/DELETE matched no row" into notFound.
func mustAffectOne(res sql.Result, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound
	}
	return nil
}
