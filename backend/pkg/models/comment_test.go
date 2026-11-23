// Owner: B
package models

import (
	"errors"
	"testing"
)

// Uses the helpers from post_test.go (openPostTestDB, postTestUser, ...).

func TestCreateAndListComments(t *testing.T) {
	db := openPostTestDB(t)
	author := postTestUser(t, db, "author")
	other := postTestUser(t, db, "other")
	post := mustCreate(t, db, author, nil, PrivacyPublic, nil)

	img := "uploads/c.gif"
	c1, err := CreateComment(db, post.ID, other, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := CreateComment(db, post.ID, author, "second", &img)
	if err != nil {
		t.Fatal(err)
	}

	if c1.PostID != post.ID || c1.Author.ID != other || c1.Author.FirstName != "other" ||
		c1.Content != "first" || c1.ImagePath != nil || c1.CreatedAt.IsZero() {
		t.Errorf("unexpected comment: %+v", c1)
	}
	if c2.ImagePath == nil || *c2.ImagePath != img {
		t.Errorf("image path not kept: %+v", c2)
	}

	list, err := CommentsByPost(db, other, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != c1.ID || list[1].ID != c2.ID {
		t.Errorf("comments = %+v, want [first, second] oldest first", list)
	}

	// the comment counter on the post follows
	got, _ := GetPost(db, post.ID)
	if got.CommentCount != 2 {
		t.Errorf("comment_count = %d, want 2", got.CommentCount)
	}
}

func TestCommentsOnHiddenPost(t *testing.T) {
	db := openPostTestDB(t)
	author := postTestUser(t, db, "author")
	follower := postTestUser(t, db, "follower")
	stranger := postTestUser(t, db, "stranger")
	postTestFollow(t, db, follower, author, "accepted")
	post := mustCreate(t, db, author, nil, PrivacyAlmostPrivate, nil)

	// a stranger can neither write nor read: both look like "not found"
	if _, err := CreateComment(db, post.ID, stranger, "hi", nil); !errors.Is(err, ErrPostNotFound) {
		t.Errorf("create as stranger: got %v, want ErrPostNotFound", err)
	}
	if _, err := CommentsByPost(db, stranger, post.ID); !errors.Is(err, ErrPostNotFound) {
		t.Errorf("list as stranger: got %v, want ErrPostNotFound", err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&n)
	if n != 0 {
		t.Errorf("%d comments stored by a stranger, want 0", n)
	}

	// the follower can do both
	if _, err := CreateComment(db, post.ID, follower, "hi", nil); err != nil {
		t.Errorf("create as follower: %v", err)
	}
	if list, err := CommentsByPost(db, follower, post.ID); err != nil || len(list) != 1 {
		t.Errorf("list as follower: %v, %v", list, err)
	}

	// a post that does not exist gives the same error
	if _, err := CreateComment(db, 9999, follower, "hi", nil); !errors.Is(err, ErrPostNotFound) {
		t.Errorf("missing post: got %v, want ErrPostNotFound", err)
	}
	if _, err := CommentsByPost(db, follower, 9999); !errors.Is(err, ErrPostNotFound) {
		t.Errorf("missing post: got %v, want ErrPostNotFound", err)
	}
}

func TestCommentsOnGroupPost(t *testing.T) {
	db := openPostTestDB(t)
	author := postTestUser(t, db, "author")
	member := postTestUser(t, db, "member")
	outsider := postTestUser(t, db, "outsider")
	postTestExec(t, db, `INSERT INTO groups (creator_id, title, description) VALUES (?, 'g', 'd')`, author)
	postTestExec(t, db, `INSERT INTO group_members (group_id, user_id, status) VALUES (1, ?, 'member')`, author)
	postTestExec(t, db, `INSERT INTO group_members (group_id, user_id, status) VALUES (1, ?, 'member')`, member)
	g := int64(1)
	post := mustCreate(t, db, author, &g, PrivacyPublic, nil)

	if _, err := CreateComment(db, post.ID, member, "ok", nil); err != nil {
		t.Errorf("member: %v", err)
	}
	if _, err := CreateComment(db, post.ID, outsider, "no", nil); !errors.Is(err, ErrPostNotFound) {
		t.Errorf("outsider create: got %v, want ErrPostNotFound", err)
	}
	if _, err := CommentsByPost(db, outsider, post.ID); !errors.Is(err, ErrPostNotFound) {
		t.Errorf("outsider list: got %v, want ErrPostNotFound", err)
	}
}

func TestCommentsEmptyListIsNotNil(t *testing.T) {
	db := openPostTestDB(t)
	author := postTestUser(t, db, "author")
	post := mustCreate(t, db, author, nil, PrivacyPublic, nil)

	list, err := CommentsByPost(db, author, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("got %#v, want empty non-nil slice", list)
	}
}
