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

func postIDs(posts []Post) []int64 {
	ids := []int64{}
	for _, p := range posts {
		ids = append(ids, p.ID)
	}
	return ids
}

func sameIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// feedFixture: "author" writes one post of each kind; "me" is the viewer.
type feedFixture struct {
	db                           *sql.DB
	author, me, other            int64
	pub, almost, priv, grp, mine *Post
}

func newFeedFixture(t *testing.T, meFollowsAuthor, meChosen bool) *feedFixture {
	t.Helper()
	db := openPostTestDB(t)
	f := &feedFixture{db: db}
	f.author = postTestUser(t, db, "author")
	f.me = postTestUser(t, db, "me")
	f.other = postTestUser(t, db, "other")
	postTestExec(t, db, `INSERT INTO groups (creator_id, title, description) VALUES (?, 'g', 'd')`, f.author)
	postTestExec(t, db, `INSERT INTO group_members (group_id, user_id, status) VALUES (1, ?, 'member')`, f.author)

	// the author needs "me" as accepted follower to choose them for a private post
	postTestFollow(t, db, f.me, f.author, "accepted")
	var viewers []int64
	if meChosen {
		viewers = []int64{f.me}
	} else {
		postTestFollow(t, db, f.other, f.author, "accepted")
		viewers = []int64{f.other}
	}
	f.pub = mustCreate(t, db, f.author, nil, PrivacyPublic, nil)
	f.almost = mustCreate(t, db, f.author, nil, PrivacyAlmostPrivate, nil)
	f.priv = mustCreate(t, db, f.author, nil, PrivacyPrivate, viewers)
	g := int64(1)
	f.grp = mustCreate(t, db, f.author, &g, PrivacyPublic, nil)
	f.mine = mustCreate(t, db, f.me, nil, PrivacyPrivate, viewersForMine(t, db, f))
	if !meFollowsAuthor {
		postTestExec(t, db, `DELETE FROM follows WHERE follower_id = ? AND following_id = ?`, f.me, f.author)
	}
	return f
}

// viewersForMine returns a valid viewer list for a private post written by "me".
func viewersForMine(t *testing.T, db *sql.DB, f *feedFixture) []int64 {
	t.Helper()
	postTestFollow(t, db, f.other, f.me, "accepted")
	return []int64{f.other}
}

func TestFeedContents(t *testing.T) {
	t.Run("follower chosen for the private post", func(t *testing.T) {
		f := newFeedFixture(t, true, true)
		got, _, err := Feed(f.db, f.me, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		// newest first; the group post is never in the feed
		want := []int64{f.mine.ID, f.priv.ID, f.almost.ID, f.pub.ID}
		if !sameIDs(postIDs(got), want) {
			t.Errorf("feed = %v, want %v", postIDs(got), want)
		}
	})

	t.Run("follower not chosen", func(t *testing.T) {
		f := newFeedFixture(t, true, false)
		got, _, _ := Feed(f.db, f.me, 0, 0)
		want := []int64{f.mine.ID, f.almost.ID, f.pub.ID}
		if !sameIDs(postIDs(got), want) {
			t.Errorf("feed = %v, want %v", postIDs(got), want)
		}
	})

	t.Run("not a follower", func(t *testing.T) {
		f := newFeedFixture(t, false, false)
		got, _, _ := Feed(f.db, f.me, 0, 0)
		want := []int64{f.mine.ID, f.pub.ID}
		if !sameIDs(postIDs(got), want) {
			t.Errorf("feed = %v, want %v", postIDs(got), want)
		}
	})

	t.Run("pending follow request does not count", func(t *testing.T) {
		f := newFeedFixture(t, false, false)
		postTestFollow(t, f.db, f.me, f.author, "pending")
		got, _, _ := Feed(f.db, f.me, 0, 0)
		want := []int64{f.mine.ID, f.pub.ID}
		if !sameIDs(postIDs(got), want) {
			t.Errorf("feed = %v, want %v", postIDs(got), want)
		}
	})
}

// Agreed with the team: public posts of a PRIVATE profile are in everyone's feed.
func TestFeedShowsPublicPostOfPrivateProfile(t *testing.T) {
	f := newFeedFixture(t, false, false)
	postTestExec(t, f.db, `UPDATE users SET is_private = 1 WHERE id = ?`, f.author)
	got, _, _ := Feed(f.db, f.me, 0, 0)
	want := []int64{f.mine.ID, f.pub.ID}
	if !sameIDs(postIDs(got), want) {
		t.Errorf("feed = %v, want %v", postIDs(got), want)
	}
}

// The SQL filter and CanViewPost must never disagree.
func TestFeedMatchesCanViewPost(t *testing.T) {
	for _, tc := range []struct{ follows, chosen bool }{{true, true}, {true, false}, {false, false}} {
		f := newFeedFixture(t, tc.follows, tc.chosen)
		inFeed := map[int64]bool{}
		got, _, _ := Feed(f.db, f.me, 0, 0)
		for _, p := range got {
			inFeed[p.ID] = true
		}
		for _, p := range []*Post{f.pub, f.almost, f.priv, f.mine} {
			can, err := CanViewPost(f.db, f.me, p)
			if err != nil {
				t.Fatal(err)
			}
			if can != inFeed[p.ID] {
				t.Errorf("post %d (%s): CanViewPost=%v but inFeed=%v", p.ID, p.Privacy, can, inFeed[p.ID])
			}
		}
	}
}

func TestFeedPaging(t *testing.T) {
	db := openPostTestDB(t)
	me := postTestUser(t, db, "me")
	var ids []int64
	for i := 0; i < 5; i++ {
		ids = append(ids, mustCreate(t, db, me, nil, PrivacyPublic, nil).ID)
	}

	page1, more, err := Feed(db, me, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !sameIDs(postIDs(page1), []int64{ids[4], ids[3]}) || !more {
		t.Errorf("page 1 = %v more=%v", postIDs(page1), more)
	}
	page2, more, _ := Feed(db, me, 2, page1[len(page1)-1].ID)
	if !sameIDs(postIDs(page2), []int64{ids[2], ids[1]}) || !more {
		t.Errorf("page 2 = %v more=%v", postIDs(page2), more)
	}
	page3, more, _ := Feed(db, me, 2, page2[len(page2)-1].ID)
	if !sameIDs(postIDs(page3), []int64{ids[0]}) || more {
		t.Errorf("page 3 = %v more=%v", postIDs(page3), more)
	}

	// an empty feed is an empty slice, not nil (JSON "[]", not "null")
	nobody := postTestUser(t, db, "nobody")
	empty, _, _ := PostsByUser(db, nobody, nobody, 10, 0)
	if empty == nil || len(empty) != 0 {
		t.Errorf("empty result = %#v, want empty non-nil slice", empty)
	}

	// limit is clamped to MaxPageSize
	for i := 0; i < MaxPageSize+5; i++ {
		mustCreate(t, db, me, nil, PrivacyPublic, nil)
	}
	big, more, _ := Feed(db, me, 1000, 0)
	if len(big) != MaxPageSize || !more {
		t.Errorf("got %d posts more=%v, want %d and more", len(big), more, MaxPageSize)
	}
}

func TestPostsByUser(t *testing.T) {
	f := newFeedFixture(t, true, false)
	// a post by someone else must never show up in the author's list
	mustCreate(t, f.db, f.other, nil, PrivacyPublic, nil)

	got, _, err := PostsByUser(f.db, f.me, f.author, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// follower, not chosen: public + almost_private, no private, no group post
	want := []int64{f.almost.ID, f.pub.ID}
	if !sameIDs(postIDs(got), want) {
		t.Errorf("posts by author = %v, want %v", postIDs(got), want)
	}

	// the author sees all of their own non-group posts
	own, _, _ := PostsByUser(f.db, f.author, f.author, 0, 0)
	want = []int64{f.priv.ID, f.almost.ID, f.pub.ID}
	if !sameIDs(postIDs(own), want) {
		t.Errorf("own posts = %v, want %v", postIDs(own), want)
	}
}
