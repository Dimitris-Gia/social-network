// Owner: B
// The profile gate for "GET /api/users/{id}/posts". SQL only.
//
// Placeholder: it reads users and follows directly, with names that cannot
// clash with A's files. If A later writes an equivalent profile check, the
// handler can switch to it.

package models

import (
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrAuthorNotFound       = errors.New("user not found")
	ErrAuthorProfilePrivate = errors.New("profile is private")
)

// CanViewAuthorPosts decides whether viewerID may look at the posts list on
// authorID's profile at all. This is the PROFILE rule; the per-post rule
// (CanViewPost) still filters the list afterwards.
//
//	unknown user                          -> ErrAuthorNotFound
//	public profile                        -> nil
//	private profile, viewer is the owner  -> nil
//	private profile, accepted follower    -> nil
//	private profile, anyone else          -> ErrAuthorProfilePrivate
func CanViewAuthorPosts(db *sql.DB, viewerID, authorID int64) error {
	var private bool
	err := db.QueryRow(`SELECT is_private FROM users WHERE id = ?`, authorID).Scan(&private)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAuthorNotFound
	}
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}
	if !private || viewerID == authorID {
		return nil
	}

	follows, err := postIsFollowing(db, viewerID, authorID)
	if err != nil {
		return err
	}
	if !follows {
		return ErrAuthorProfilePrivate
	}
	return nil
}
