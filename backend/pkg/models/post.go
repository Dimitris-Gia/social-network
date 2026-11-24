// Owner: B
// CreatePost, GetPost, CanViewPost. SQL only: no HTTP code in this file.
//
// Everything that is not B's own table (follows, group_members) is read with a
// small private query in this file, prefixed "post", so it cannot clash with
// the exported helpers A and C write in follow.go and group.go. Once those
// exist, the private helpers can be replaced by calls to them.

package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// The three privacy levels of a post (posts.privacy).
const (
	PrivacyPublic        = "public"         // every logged-in user
	PrivacyAlmostPrivate = "almost_private" // the author's accepted followers
	PrivacyPrivate       = "private"        // only the users in post_allowed_viewers
)

var (
	ErrPostNotFound      = errors.New("post not found")
	ErrInvalidPrivacy    = errors.New("invalid privacy value")
	ErrViewerNotFollower = errors.New("allowed viewer does not follow the author")
	ErrNoViewers         = errors.New("a private post needs at least one allowed viewer")
)

// PostAuthor is the small user object embedded in a post (see API.md, "UserSummary").
type PostAuthor struct {
	ID         int64   `json:"id"`
	FirstName  string  `json:"first_name"`
	LastName   string  `json:"last_name"`
	Nickname   *string `json:"nickname"`
	AvatarPath *string `json:"avatar_path"`
}

// Post is the shape described in API.md. ImagePath is the URL path returned by
// utils.SaveImage (e.g. "/uploads/<uuid>.png"). It is stored and sent as is.
type Post struct {
	ID           int64      `json:"id"`
	Author       PostAuthor `json:"author"`
	GroupID      *int64     `json:"group_id"`
	Content      string     `json:"content"`
	ImagePath    *string    `json:"image_path"`
	Privacy      string     `json:"privacy"`
	CommentCount int        `json:"comment_count"`
	CreatedAt    time.Time  `json:"created_at"`
}

// rowQuerier is satisfied by both *sql.DB and *sql.Tx, so the same helper
// works inside and outside a transaction.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// CreatePost inserts a post and, for a private post, its allowed viewers, in
// one transaction. groupID is nil for a normal post. Every allowed viewer must
// be an accepted follower of the author.
func CreatePost(db *sql.DB, userID int64, groupID *int64, content string, imagePath *string, privacy string, allowedViewers []int64) (int64, error) {
	switch privacy {
	case PrivacyPublic, PrivacyAlmostPrivate, PrivacyPrivate:
	default:
		return 0, ErrInvalidPrivacy
	}
	needsViewers := privacy == PrivacyPrivate && groupID == nil
	if needsViewers && len(allowedViewers) == 0 {
		return 0, ErrNoViewers
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() // no-op after a successful Commit

	res, err := tx.Exec(
		`INSERT INTO posts (user_id, group_id, content, image_path, privacy) VALUES (?, ?, ?, ?, ?)`,
		userID, groupID, content, imagePath, privacy,
	)
	if err != nil {
		return 0, fmt.Errorf("insert post: %w", err)
	}
	postID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("post id: %w", err)
	}

	if needsViewers {
		for _, viewerID := range allowedViewers {
			follows, err := postIsFollowing(tx, viewerID, userID)
			if err != nil {
				return 0, err
			}
			if !follows {
				return 0, ErrViewerNotFollower
			}
			// OR IGNORE: the same ID listed twice is not an error.
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO post_allowed_viewers (post_id, user_id) VALUES (?, ?)`,
				postID, viewerID,
			); err != nil {
				return 0, fmt.Errorf("insert allowed viewer: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return postID, nil
}

// postColumns and scanPost keep the SELECT list and the Scan call in one
// place, so GetPost, Feed and PostsByUser always agree.
const postColumns = `p.id, p.group_id, p.content, p.image_path, p.privacy, p.created_at,
	u.id, u.first_name, u.last_name, u.nickname, u.avatar_path,
	(SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)`

// rowScanner is satisfied by *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanPost(r rowScanner) (Post, error) {
	var p Post
	err := r.Scan(
		&p.ID, &p.GroupID, &p.Content, &p.ImagePath, &p.Privacy, &p.CreatedAt,
		&p.Author.ID, &p.Author.FirstName, &p.Author.LastName, &p.Author.Nickname, &p.Author.AvatarPath,
		&p.CommentCount,
	)
	return p, err
}

// GetPost loads one post with its author and comment count. It does NOT check
// who may see it: call CanViewPost before showing it to anyone.
func GetPost(db *sql.DB, id int64) (*Post, error) {
	p, err := scanPost(db.QueryRow(
		`SELECT `+postColumns+` FROM posts p JOIN users u ON u.id = p.user_id WHERE p.id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPostNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get post: %w", err)
	}
	return &p, nil
}

// CanViewPost is the single visibility rule for posts (and, through them, for
// comments). Call it on EVERY read, not only in the feed.
//
//	own post                -> always
//	group post              -> members of that group only (privacy is ignored)
//	public                  -> everyone
//	almost_private          -> accepted followers of the author
//	private                 -> users listed in post_allowed_viewers
func CanViewPost(db *sql.DB, viewerID int64, p *Post) (bool, error) {
	if p.Author.ID == viewerID {
		return true, nil
	}
	if p.GroupID != nil {
		return postIsGroupMember(db, *p.GroupID, viewerID)
	}
	switch p.Privacy {
	case PrivacyPublic:
		return true, nil
	case PrivacyAlmostPrivate:
		return postIsFollowing(db, viewerID, p.Author.ID)
	case PrivacyPrivate:
		var ok bool
		err := db.QueryRow(
			`SELECT EXISTS (SELECT 1 FROM post_allowed_viewers WHERE post_id = ? AND user_id = ?)`,
			p.ID, viewerID,
		).Scan(&ok)
		if err != nil {
			return false, fmt.Errorf("check allowed viewer: %w", err)
		}
		return ok, nil
	}
	return false, nil // unknown privacy value: deny
}

// postIsFollowing reports whether followerID has an ACCEPTED follow on
// followingID. Placeholder for A's IsFollowing in follow.go.
func postIsFollowing(q rowQuerier, followerID, followingID int64) (bool, error) {
	var ok bool
	err := q.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM follows WHERE follower_id = ? AND following_id = ? AND status = 'accepted')`,
		followerID, followingID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check follow: %w", err)
	}
	return ok, nil
}

// postIsGroupMember reports whether userID is a full member (not just invited
// or requested) of the group. Placeholder for C's IsMember in group.go.
func postIsGroupMember(q rowQuerier, groupID, userID int64) (bool, error) {
	var ok bool
	err := q.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = ? AND user_id = ? AND status = 'member')`,
		groupID, userID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check membership: %w", err)
	}
	return ok, nil
}

// Page sizes for Feed and PostsByUser (see API.md).
const (
	DefaultPageSize = 20
	MaxPageSize     = 50
)

// Feed returns the posts the viewer may see on their home page, newest first:
// their own posts, public posts, almost_private posts of users they follow,
// and private posts they were chosen for. Group posts are never in the feed.
//
// before is a post ID: only older posts are returned (0 = start from the
// newest). hasMore tells the frontend whether another page exists.
func Feed(db *sql.DB, viewerID int64, limit int, before int64) (posts []Post, hasMore bool, err error) {
	return listPosts(db, viewerID, 0, limit, before)
}

// PostsByUser returns the posts of authorID that the viewer may see, newest
// first, with the same paging as Feed. It applies the POST rule only. Whether
// the viewer may open authorID's profile at all (private profile gate, 403 in
// API.md) is checked by the handler before calling this.
func PostsByUser(db *sql.DB, viewerID, authorID int64, limit int, before int64) (posts []Post, hasMore bool, err error) {
	return listPosts(db, viewerID, authorID, limit, before)
}

// listPosts is the one query behind Feed and PostsByUser. The visibility rules
// are the same as CanViewPost, written as SQL so a page is filtered in the
// database. If you change one, change the other (the tests compare them).
//
// authorID = 0 means "any author"; before = 0 means "no upper bound". Both use
// the "? = 0 OR ..." form so the SQL text is constant: values only ever go
// through ? placeholders.
func listPosts(db *sql.DB, viewerID, authorID int64, limit int, before int64) ([]Post, bool, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}

	rows, err := db.Query(`
		SELECT `+postColumns+`
		FROM posts p
		JOIN users u ON u.id = p.user_id
		WHERE p.group_id IS NULL
		  AND (? = 0 OR p.user_id = ?)
		  AND (? = 0 OR p.id < ?)
		  AND (
		        p.user_id = ?
		     OR p.privacy = 'public'
		     OR (p.privacy = 'almost_private' AND EXISTS (
		            SELECT 1 FROM follows f
		            WHERE f.follower_id = ? AND f.following_id = p.user_id AND f.status = 'accepted'))
		     OR (p.privacy = 'private' AND EXISTS (
		            SELECT 1 FROM post_allowed_viewers v
		            WHERE v.post_id = p.id AND v.user_id = ?))
		  )
		ORDER BY p.id DESC
		LIMIT ?`,
		authorID, authorID,
		before, before,
		viewerID, viewerID, viewerID,
		limit+1, // one extra row tells us whether there is a next page
	)
	if err != nil {
		return nil, false, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	posts := []Post{} // empty slice, not nil, so JSON is [] and not null
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list posts: %w", err)
	}

	hasMore := len(posts) > limit
	if hasMore {
		posts = posts[:limit]
	}
	return posts, hasMore, nil
}
