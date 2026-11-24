// Owner: B
package models

import (
	"errors"
	"testing"
)

func TestCanViewAuthorPosts(t *testing.T) {
	db := openPostTestDB(t)
	owner := postTestUser(t, db, "owner")
	follower := postTestUser(t, db, "follower")
	pending := postTestUser(t, db, "pending")
	stranger := postTestUser(t, db, "stranger")
	postTestFollow(t, db, follower, owner, "accepted")
	postTestFollow(t, db, pending, owner, "pending")

	check := func(name string, viewer, author int64, want error) {
		t.Helper()
		if got := CanViewAuthorPosts(db, viewer, author); !errors.Is(got, want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}

	// public profile: everyone
	check("public, stranger", stranger, owner, nil)

	postTestExec(t, db, `UPDATE users SET is_private = 1 WHERE id = ?`, owner)
	check("private, owner", owner, owner, nil)
	check("private, accepted follower", follower, owner, nil)
	check("private, pending follower", pending, owner, ErrAuthorProfilePrivate)
	check("private, stranger", stranger, owner, ErrAuthorProfilePrivate)

	check("unknown user", stranger, 9999, ErrAuthorNotFound)
}
