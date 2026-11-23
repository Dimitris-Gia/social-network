// Owner: C
// CreateGroup, ListGroups, GetGroup, IsMember,
// Invite, RequestJoin, AcceptMember, DeclineMember, Members

package models

import (
	"database/sql"
	"errors"
	"time"
)

var ErrGroupConflict = errors.New("group membership already exists")

// Group describes a group returned to the frontend.
type Group struct {
	ID          int64     `json:"id"`
	CreatorID   int64     `json:"creator_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status,omitempty"`
	MemberCount int       `json:"member_count,omitempty"`
}

// Member describes a confirmed member or a pending invitation/request.
type Member struct {
	UserID     int64   `json:"user_id"`
	FirstName  string  `json:"first_name"`
	LastName   string  `json:"last_name"`
	Nickname   *string `json:"nickname,omitempty"`
	AvatarPath *string `json:"avatar_path,omitempty"`
	Status     string  `json:"status"`
	InvitedBy  *int64  `json:"invited_by,omitempty"`
}

// CreateGroup creates the group and adds the creator as a confirmed member.
func CreateGroup(db *sql.DB, creatorID int64, title, description string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`INSERT INTO groups (creator_id, title, description) VALUES (?, ?, ?)`, creatorID, title, description)
	if err != nil {
		return 0, err
	}
	groupID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`INSERT INTO group_members (group_id, user_id, status) VALUES (?, ?, 'member')`, groupID, creatorID); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return groupID, nil
}

// ListGroups lists every group and, if viewerID is present, the viewer's status.
func ListGroups(db *sql.DB, viewerID ...int64) ([]Group, error) {
	var viewer int64
	if len(viewerID) > 0 {
		viewer = viewerID[0]
	}

	rows, err := db.Query(`
		SELECT g.id, g.creator_id, g.title, g.description, g.created_at,
		       COALESCE(gm.status, 'none'),
		       (SELECT COUNT(*) FROM group_members mc
		        WHERE mc.group_id = g.id AND mc.status = 'member')
		FROM groups g
		LEFT JOIN group_members gm ON gm.group_id = g.id AND gm.user_id = ?
		ORDER BY g.created_at DESC, g.id DESC`, viewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]Group, 0)
	for rows.Next() {
		var group Group
		if err := rows.Scan(&group.ID, &group.CreatorID, &group.Title, &group.Description,
			&group.CreatedAt, &group.Status, &group.MemberCount); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

// GetGroup returns one group. It does not perform a membership check.
func GetGroup(db *sql.DB, groupID int64) (*Group, error) {
	var group Group
	err := db.QueryRow(`
		SELECT g.id, g.creator_id, g.title, g.description, g.created_at,
		       (SELECT COUNT(*) FROM group_members mc
		        WHERE mc.group_id = g.id AND mc.status = 'member')
		FROM groups g WHERE g.id = ?`, groupID).
		Scan(&group.ID, &group.CreatorID, &group.Title, &group.Description,
			&group.CreatedAt, &group.MemberCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// IsMember reports whether a user has confirmed membership in a group.
func IsMember(db *sql.DB, userID, groupID int64) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM group_members
		WHERE group_id = ? AND user_id = ? AND status = 'member'`, groupID, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 1, nil
}

// Invite creates an invitation. The inviter must be a member; the invitee
// must follow the inviter. Those checks are also enforced in the handler.
func Invite(db *sql.DB, groupID, inviterID, inviteeID int64) error {
	var existing string
	err := db.QueryRow(`SELECT status FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, inviteeID).Scan(&existing)
	if err == nil {
		return ErrGroupConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	_, err = db.Exec(`INSERT INTO group_members (group_id, user_id, status, invited_by)
		VALUES (?, ?, 'invited', ?)`, groupID, inviteeID, inviterID)
	if err != nil {
		return err
	}
	return nil
}

// RequestJoin creates a requested status for a user who does not belong yet.
func RequestJoin(db *sql.DB, groupID, userID int64) error {
	var existing string
	err := db.QueryRow(`SELECT status FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, userID).Scan(&existing)
	if err == nil {
		return ErrGroupConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	_, err = db.Exec(`INSERT INTO group_members (group_id, user_id, status) VALUES (?, ?, 'requested')`, groupID, userID)
	return err
}

// AcceptMember changes an invitation or join request into confirmed membership.
func AcceptMember(db *sql.DB, groupID, userID int64) error {
	result, err := db.Exec(`UPDATE group_members SET status = 'member'
		WHERE group_id = ? AND user_id = ? AND status IN ('invited', 'requested')`, groupID, userID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeclineMember removes an invitation or join request.
func DeclineMember(db *sql.DB, groupID, userID int64) error {
	result, err := db.Exec(`DELETE FROM group_members
		WHERE group_id = ? AND user_id = ? AND status IN ('invited', 'requested')`, groupID, userID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Members returns all users in the group, including pending states.
func Members(db *sql.DB, groupID int64) ([]Member, error) {
	rows, err := db.Query(`
		SELECT gm.user_id, u.first_name, u.last_name, u.nickname, u.avatar_path,
		       gm.status, gm.invited_by
		FROM group_members gm
		JOIN users u ON u.id = gm.user_id
		WHERE gm.group_id = ?
		ORDER BY CASE gm.status WHEN 'member' THEN 0 ELSE 1 END,
		         u.first_name, u.last_name`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]Member, 0)
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.UserID, &member.FirstName, &member.LastName,
			&member.Nickname, &member.AvatarPath, &member.Status, &member.InvitedBy); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}
