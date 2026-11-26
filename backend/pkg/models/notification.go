// Owner: A
// CreateNotification, List, UnreadCount, MarkRead
//
// A notification row says "something happened that user_id should know
// about". actor_id / group_id / event_id point at what it's about; the list
// query joins them so the frontend gets names and titles in one request.
//
// Request-type notifications (follow_request, group_invite,
// group_join_request) are DELETED as soon as the request is answered or
// cancelled. So every one still in the list is still open, and the bell can
// always show Accept / Decline without asking "is this still pending?".

package models

import (
	"database/sql"
	"errors"
	"time"
)

// Notification types. They must match the CHECK list in migration 000012.
const (
	NotifFollowRequest    = "follow_request"
	NotifGroupInvite      = "group_invite"
	NotifGroupJoinRequest = "group_join_request"
	NotifGroupEvent       = "group_event"
	NotifFollowAccepted   = "follow_accepted" // bonus, week 3
	NotifNewFollower      = "new_follower"    // bonus, week 3
)

// notificationListLimit is how many notifications GET /api/notifications returns.
const notificationListLimit = 50

var ErrNotificationNotFound = errors.New("notification not found")

// NotifGroup and NotifEvent are the small objects embedded in a notification.
type NotifGroup struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type NotifEvent struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// Notification is the shape in API.md 1.4. Pointers become JSON null when
// the notification isn't about a user / group / event.
type Notification struct {
	ID        int64        `json:"id"`
	Type      string       `json:"type"`
	IsRead    bool         `json:"is_read"`
	CreatedAt time.Time    `json:"created_at"`
	Actor     *UserSummary `json:"actor"`
	Group     *NotifGroup  `json:"group"`
	Event     *NotifEvent  `json:"event"`
}

// NewNotification is what callers fill in to create one. GroupID and
// EventID are 0 when they don't apply (stored as NULL).
type NewNotification struct {
	UserID  int64 // who receives it
	ActorID int64 // who caused it
	Type    string
	GroupID int64
	EventID int64
}

// notificationSelect loads notifications with their actor, group and event.
// LEFT JOIN, because group and event are NULL for follow notifications.
const notificationSelect = `SELECT n.id, n.type, n.is_read, n.created_at,
		a.id, a.first_name, a.last_name, a.nickname, a.avatar_path,
		g.id, g.title, e.id, e.title
	FROM notifications n
	LEFT JOIN users  a ON a.id = n.actor_id
	LEFT JOIN groups g ON g.id = n.group_id
	LEFT JOIN events e ON e.id = n.event_id`

// nullID turns 0 into NULL for the optional foreign keys.
func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// CreateNotification inserts one notification and returns its ID.
func CreateNotification(db *sql.DB, n NewNotification) (int64, error) {
	res, err := db.Exec(`INSERT INTO notifications (user_id, actor_id, type, group_id, event_id)
		VALUES (?, ?, ?, ?, ?)`,
		n.UserID, nullID(n.ActorID), n.Type, nullID(n.GroupID), nullID(n.EventID))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetNotification loads one notification (used to push it over the WebSocket
// right after creating it).
func GetNotification(db *sql.DB, id int64) (*Notification, error) {
	return scanNotification(db.QueryRow(notificationSelect+` WHERE n.id = ?`, id))
}

// ListNotifications returns userID's newest notifications, newest first.
func ListNotifications(db *sql.DB, userID int64) ([]Notification, error) {
	rows, err := db.Query(notificationSelect+`
		WHERE n.user_id = ?
		ORDER BY n.created_at DESC, n.id DESC
		LIMIT ?`, userID, notificationListLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Notification{}
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *n)
	}
	return list, rows.Err()
}

// UnreadNotificationCount is the number on the bell.
func UnreadNotificationCount(db *sql.DB, userID int64) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = 0`,
		userID).Scan(&n)
	return n, err
}

// MarkNotificationRead marks one notification as read. "AND user_id = ?" is
// the ownership check: someone else's ID matches no row and gives
// ErrNotificationNotFound, the same as an ID that doesn't exist, so nobody
// learns which IDs exist.
func MarkNotificationRead(db *sql.DB, id, userID int64) error {
	res, err := db.Exec(`UPDATE notifications SET is_read = 1 WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	return mustAffectOne(res, ErrNotificationNotFound)
}

// MarkAllNotificationsRead is called when the bell dropdown opens.
func MarkAllNotificationsRead(db *sql.DB, userID int64) error {
	_, err := db.Exec(`UPDATE notifications SET is_read = 1 WHERE user_id = ? AND is_read = 0`, userID)
	return err
}

// DeleteNotifications removes the notifications of one request once it has
// been answered or cancelled. actorID and groupID narrow it down; 0 means
// "any". Examples:
//
//	follow request from 5 to 3:  DeleteNotifications(db, 3, NotifFollowRequest, 5, 0)
//	all follow requests to 3:    DeleteNotifications(db, 3, NotifFollowRequest, 0, 0)
//	C, invite of 7 to group 2:   DeleteNotifications(db, 7, NotifGroupInvite, 0, 2)
//	C, join request of 7 to 2:   DeleteNotifications(db, creatorID, NotifGroupJoinRequest, 7, 2)
func DeleteNotifications(db *sql.DB, userID int64, typ string, actorID, groupID int64) error {
	_, err := db.Exec(`DELETE FROM notifications
		WHERE user_id = ? AND type = ?
		  AND (? = 0 OR actor_id = ?)
		  AND (? = 0 OR group_id = ?)`,
		userID, typ, actorID, actorID, groupID, groupID)
	return err
}

// scanNotification reads one row of notificationSelect. The joined columns
// can be NULL, so they're scanned into Null* types first.
func scanNotification(s interface{ Scan(...any) error }) (*Notification, error) {
	var (
		n                      Notification
		actorID                sql.NullInt64
		actorFirst, actorLast  sql.NullString
		actorNick, actorAvatar *string
		groupID, eventID       sql.NullInt64
		groupTitle, eventTitle sql.NullString
	)
	err := s.Scan(&n.ID, &n.Type, &n.IsRead, &n.CreatedAt,
		&actorID, &actorFirst, &actorLast, &actorNick, &actorAvatar,
		&groupID, &groupTitle, &eventID, &eventTitle)
	if err != nil {
		return nil, err
	}
	if actorID.Valid {
		n.Actor = &UserSummary{
			ID: actorID.Int64, FirstName: actorFirst.String,
			LastName: actorLast.String, Nickname: actorNick, Avatar: actorAvatar,
		}
	}
	if groupID.Valid {
		n.Group = &NotifGroup{ID: groupID.Int64, Title: groupTitle.String}
	}
	if eventID.Valid {
		n.Event = &NotifEvent{ID: eventID.Int64, Title: eventTitle.String}
	}
	return &n, nil
}
