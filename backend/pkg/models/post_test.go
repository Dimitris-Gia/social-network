// Owner: B
package models

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"social-network/pkg/db/sqlite"
)

// openPostTestDB opens a fresh database with every migration applied.
// sqlite.Open reads migrations from a path relative to backend/, so we step
// into backend/ for the call and step back out, instead of changing the
// working directory for the whole test binary (other tests may rely on it).
func openPostTestDB(t *testing.T) *sql.DB {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	backendDir := filepath.Join(filepath.Dir(thisFile), "..", "..")

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(backendDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prev)

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func postTestExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func postTestUser(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO users (email, password_hash, first_name, last_name, date_of_birth)
		VALUES (?, 'x', ?, 'Test', '2000-01-01')`, name+"@test.com", name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func postTestFollow(t *testing.T, db *sql.DB, follower, following int64, status string) {
	t.Helper()
	postTestExec(t, db, `INSERT INTO follows (follower_id, following_id, status) VALUES (?, ?, ?)`, follower, following, status)
}

func mustCreate(t *testing.T, db *sql.DB, author int64, group *int64, privacy string, viewers []int64) *Post {
	t.Helper()
	id, err := CreatePost(db, author, group, "hello", nil, privacy, viewers)
	if err != nil {
		t.Fatal(err)
	}
	p, err := GetPost(db, id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCanViewPost(t *testing.T) {
	db := openPostTestDB(t)
	author := postTestUser(t, db, "author")
	follower := postTestUser(t, db, "follower") // accepted follower
	pending := postTestUser(t, db, "pending")   // follow request not accepted
	stranger := postTestUser(t, db, "stranger") // no relation
	chosen := postTestUser(t, db, "chosen")     // follower chosen for the private post
	unchosen := postTestUser(t, db, "unchosen") // follower NOT chosen
	postTestFollow(t, db, follower, author, "accepted")
	postTestFollow(t, db, pending, author, "pending")
	postTestFollow(t, db, chosen, author, "accepted")
	postTestFollow(t, db, unchosen, author, "accepted")

	pub := mustCreate(t, db, author, nil, PrivacyPublic, nil)
	almost := mustCreate(t, db, author, nil, PrivacyAlmostPrivate, nil)
	priv := mustCreate(t, db, author, nil, PrivacyPrivate, []int64{chosen})

	tests := []struct {
		name   string
		viewer int64
		post   *Post
		want   bool
	}{
		{"author sees own public", author, pub, true},
		{"author sees own private", author, priv, true},
		{"stranger sees public", stranger, pub, true},
		{"stranger cannot see almost_private", stranger, almost, false},
		{"pending follower cannot see almost_private", pending, almost, false},
		{"accepted follower sees almost_private", follower, almost, true},
		{"stranger cannot see private", stranger, priv, false},
		{"follower not chosen cannot see private", unchosen, priv, false},
		{"chosen follower sees private", chosen, priv, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanViewPost(db, tc.viewer, tc.post)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("CanViewPost = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanViewGroupPost(t *testing.T) {
	db := openPostTestDB(t)
	author := postTestUser(t, db, "author")
	member := postTestUser(t, db, "member")
	invited := postTestUser(t, db, "invited")
	outsider := postTestUser(t, db, "outsider")
	postTestExec(t, db, `INSERT INTO groups (creator_id, title, description) VALUES (?, 'g', 'd')`, author)
	postTestExec(t, db, `INSERT INTO group_members (group_id, user_id, status) VALUES (1, ?, 'member')`, author)
	postTestExec(t, db, `INSERT INTO group_members (group_id, user_id, status) VALUES (1, ?, 'member')`, member)
	postTestExec(t, db, `INSERT INTO group_members (group_id, user_id, status) VALUES (1, ?, 'invited')`, invited)

	groupID := int64(1)
	// A public group post is still only for members: the group rule comes first.
	p := mustCreate(t, db, author, &groupID, PrivacyPublic, nil)

	for _, tc := range []struct {
		name   string
		viewer int64
		want   bool
	}{
		{"author", author, true},
		{"member", member, true},
		{"invited but not yet member", invited, false},
		{"outsider", outsider, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanViewPost(db, tc.viewer, p)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("CanViewPost = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreatePostValidation(t *testing.T) {
	db := openPostTestDB(t)
	author := postTestUser(t, db, "author")
	stranger := postTestUser(t, db, "stranger")
	follower := postTestUser(t, db, "follower")
	postTestFollow(t, db, follower, author, "accepted")

	if _, err := CreatePost(db, author, nil, "x", nil, "secret", nil); !errors.Is(err, ErrInvalidPrivacy) {
		t.Errorf("bad privacy: got %v, want ErrInvalidPrivacy", err)
	}
	if _, err := CreatePost(db, author, nil, "x", nil, PrivacyPrivate, nil); !errors.Is(err, ErrNoViewers) {
		t.Errorf("no viewers: got %v, want ErrNoViewers", err)
	}
	if _, err := CreatePost(db, author, nil, "x", nil, PrivacyPrivate, []int64{follower, stranger}); !errors.Is(err, ErrViewerNotFollower) {
		t.Errorf("non-follower viewer: got %v, want ErrViewerNotFollower", err)
	}

	// The failed attempt above must have been rolled back completely.
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&n)
	if n != 0 {
		t.Errorf("%d posts left behind after a failed CreatePost, want 0", n)
	}

	// The same viewer listed twice is fine.
	if _, err := CreatePost(db, author, nil, "x", nil, PrivacyPrivate, []int64{follower, follower}); err != nil {
		t.Errorf("duplicate viewer: %v", err)
	}
}

func TestGetPost(t *testing.T) {
	db := openPostTestDB(t)
	author := postTestUser(t, db, "author")
	img := "uploads/a.png"
	id, err := CreatePost(db, author, nil, "with image", &img, PrivacyPublic, nil)
	if err != nil {
		t.Fatal(err)
	}
	postTestExec(t, db, `INSERT INTO comments (post_id, user_id, content) VALUES (?, ?, 'c')`, id, author)

	p, err := GetPost(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if p.Author.ID != author || p.Author.FirstName != "author" || p.Content != "with image" ||
		p.ImagePath == nil || *p.ImagePath != img || p.GroupID != nil || p.CommentCount != 1 || p.CreatedAt.IsZero() {
		t.Errorf("unexpected post: %+v", p)
	}

	if _, err := GetPost(db, 9999); !errors.Is(err, ErrPostNotFound) {
		t.Errorf("missing post: got %v, want ErrPostNotFound", err)
	}
}
