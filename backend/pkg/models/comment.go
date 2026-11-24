// Owner: B
// CreateComment, CommentsByPost. SQL only: no HTTP code in this file.
//
// A comment is visible exactly when its post is visible, so both functions
// run CanViewPost themselves. A handler cannot forget the check, and group
// posts (C) get the group-membership rule for free.

package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Comment is the shape described in API.md. ImagePath is the URL path returned by
// utils.SaveImage (e.g. "/uploads/<uuid>.png"). It is stored and sent as is.
type Comment struct {
	ID        int64      `json:"id"`
	PostID    int64      `json:"post_id"`
	Author    PostAuthor `json:"author"`
	Content   string     `json:"content"`
	ImagePath *string    `json:"image_path"`
	CreatedAt time.Time  `json:"created_at"`
}

const commentColumns = `c.id, c.post_id, c.content, c.image_path, c.created_at,
	u.id, u.first_name, u.last_name, u.nickname, u.avatar_path`

func scanComment(r rowScanner) (Comment, error) {
	var c Comment
	err := r.Scan(
		&c.ID, &c.PostID, &c.Content, &c.ImagePath, &c.CreatedAt,
		&c.Author.ID, &c.Author.FirstName, &c.Author.LastName, &c.Author.Nickname, &c.Author.AvatarPath,
	)
	return c, err
}

// visiblePost loads a post and checks that viewerID may see it. A post that
// does not exist and a post the viewer may not see give the SAME error
// (ErrPostNotFound), so nobody can probe for hidden posts (404 in API.md).
func visiblePost(db *sql.DB, viewerID, postID int64) error {
	p, err := GetPost(db, postID)
	if err != nil {
		return err
	}
	ok, err := CanViewPost(db, viewerID, p)
	if err != nil {
		return err
	}
	if !ok {
		return ErrPostNotFound
	}
	return nil
}

// CreateComment adds a comment by userID on a post that userID may see, and
// returns it with its author. Returns ErrPostNotFound if the post does not
// exist or is not visible to userID.
func CreateComment(db *sql.DB, postID, userID int64, content string, imagePath *string) (*Comment, error) {
	if err := visiblePost(db, userID, postID); err != nil {
		return nil, err
	}

	res, err := db.Exec(
		`INSERT INTO comments (post_id, user_id, content, image_path) VALUES (?, ?, ?, ?)`,
		postID, userID, content, imagePath,
	)
	if err != nil {
		return nil, fmt.Errorf("insert comment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("comment id: %w", err)
	}

	c, err := scanComment(db.QueryRow(
		`SELECT `+commentColumns+` FROM comments c JOIN users u ON u.id = c.user_id WHERE c.id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("comment %d vanished after insert", id)
	}
	if err != nil {
		return nil, fmt.Errorf("load comment: %w", err)
	}
	return &c, nil
}

// CommentsByPost lists the comments of a post, oldest first. Returns
// ErrPostNotFound if the post does not exist or is not visible to viewerID.
func CommentsByPost(db *sql.DB, viewerID, postID int64) ([]Comment, error) {
	if err := visiblePost(db, viewerID, postID); err != nil {
		return nil, err
	}

	rows, err := db.Query(
		`SELECT `+commentColumns+`
		 FROM comments c
		 JOIN users u ON u.id = c.user_id
		 WHERE c.post_id = ?
		 ORDER BY c.id ASC`, postID,
	)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	comments := []Comment{} // empty slice, not nil, so JSON is [] and not null
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	return comments, nil
}
